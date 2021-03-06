package main

import (
	"io"
	"io/ioutil"
	"net"
	"strconv"

	. "github.com/flynn/flynn/Godeps/_workspace/src/gopkg.in/check.v1"
	"github.com/flynn/flynn/discoverd/client/testutil"
	"github.com/flynn/flynn/router/types"
)

func NewTCPTestServer(prefix string) *TCPTestServer {
	s := &TCPTestServer{prefix: prefix}
	var err error
	s.l, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	s.Addr = s.l.Addr().String()
	go s.Serve()
	return s
}

type TCPTestServer struct {
	Addr   string
	prefix string
	l      net.Listener
}

func (s *TCPTestServer) Serve() {
	for {
		conn, err := s.l.Accept()
		if err != nil {
			return
		}
		go func() {
			conn.Write([]byte(s.prefix))
			io.Copy(conn, conn)
			conn.Close()
		}()
	}
}

func (s *TCPTestServer) Close() error { return s.l.Close() }

const firstTCPPort, lastTCPPort = 10001, 10010

type tcpListener struct {
	*TCPListener
	cleanup func()
}

func (l *tcpListener) Close() error {
	l.TCPListener.Close()
	if l.cleanup != nil {
		l.cleanup()
	}
	return nil
}

func newTCPListenerClients(t testutil.TestingT, etcd EtcdClient, discoverd discoverdClient) (*tcpListener, discoverdClient) {
	discoverd, etcd, cleanup := setup(t, etcd, discoverd)
	l := &tcpListener{
		NewTCPListener("127.0.0.1", firstTCPPort, lastTCPPort, NewEtcdDataStore(etcd, "/strowger/tcp/"), discoverd),
		cleanup,
	}
	if err := l.Start(); err != nil {
		t.Fatal(err)
	}
	return l, discoverd
}

func newTCPListener(t testutil.TestingT) (*tcpListener, discoverdClient) {
	return newTCPListenerClients(t, nil, nil)
}

func assertTCPConn(c *C, addr, prefix string) {
	conn, err := net.Dial("tcp", addr)
	c.Assert(err, IsNil)
	conn.Write([]byte("asdf"))
	conn.(*net.TCPConn).CloseWrite()
	res, err := ioutil.ReadAll(conn)
	conn.Close()

	c.Assert(err, IsNil)
	c.Assert(string(res), Equals, prefix+"asdf")
}

func (s *S) TestAddTCPRoute(c *C) {
	const addr, port, portInt = "127.0.0.1:45000", "45000", 45000
	srv1 := NewTCPTestServer("1")
	srv2 := NewTCPTestServer("2")
	defer srv1.Close()
	defer srv2.Close()

	l, discoverd := newTCPListener(c)
	defer l.Close()

	discoverdRegister(c, discoverd, srv1.Addr)
	defer discoverd.UnregisterAll()

	r := addTCPRoute(c, l, portInt)

	assertTCPConn(c, addr, "1")

	discoverd.Unregister("test", srv1.Addr)
	discoverdRegister(c, discoverd, srv2.Addr)

	assertTCPConn(c, addr, "2")

	wait := waitForEvent(c, l, "remove", r.Route.ID)
	err := l.RemoveRoute(r.Route.ID)
	c.Assert(err, IsNil)
	wait()

	_, err = net.Dial("tcp", addr)
	c.Assert(err, Not(IsNil))
}

func addTCPRoute(c *C, l *tcpListener, port int) *strowger.TCPRoute {
	wait := waitForEvent(c, l, "set", "")
	r := (&strowger.TCPRoute{
		Service: "test",
		Port:    port,
	}).ToRoute()
	err := l.AddRoute(r)
	c.Assert(err, IsNil)
	wait()
	return r.TCPRoute()
}

func (s *S) TestInitialTCPSync(c *C) {
	const addr, port = "127.0.0.1:45000", 45000
	etcd, cleanup := newEtcd(c)
	defer cleanup()
	l, _ := newTCPListenerClients(c, etcd, nil)
	addTCPRoute(c, l, port)
	l.Close()

	srv := NewTCPTestServer("1")
	defer srv.Close()

	l, discoverd := newTCPListenerClients(c, etcd, nil)
	defer l.Close()

	discoverdRegister(c, discoverd, srv.Addr)
	defer discoverd.UnregisterAll()

	assertTCPConn(c, addr, "1")
}

func (s *S) TestTCPPortAllocation(c *C) {
	l, discoverd := newTCPListener(c)
	defer l.Close()
	for i := 0; i < 2; i++ {
		ports := make([]string, 0, 10)
		for j := 0; j < 10; j++ {
			route := addTCPRoute(c, l, 0)
			c.Assert(route.Port >= firstTCPPort && route.Port <= lastTCPPort, Equals, true)

			port := strconv.Itoa(route.Port)
			ports = append(ports, route.ID)
			srv := NewTCPTestServer(port)
			discoverdRegister(c, discoverd, srv.Addr)

			assertTCPConn(c, "127.0.0.1:"+port, port)
			discoverd.UnregisterAll()
			srv.Close()
		}
		r := (&strowger.TCPRoute{Service: "test"}).ToRoute()
		err := l.AddRoute(r)
		c.Assert(err, Equals, ErrNoPorts)
		for _, port := range ports {
			wait := waitForEvent(c, l, "remove", port)
			l.RemoveRoute(port)
			wait()
		}
	}
}
