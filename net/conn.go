package net

import (
	"net"

	log "github.com/sirupsen/logrus"
	"github.com/xvzc/SpoofDPI/doh"
	"github.com/xvzc/SpoofDPI/packet"
)

const BUF_SIZE = 1024

type Conn struct {
	Conn net.Conn
}

func (conn *Conn) Close() {
	conn.Conn.Close()
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.Conn.RemoteAddr()
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.Conn.LocalAddr()
}

func (conn *Conn) Read(b []byte) (n int, err error) {
	return conn.Conn.Read(b)
}

func (conn *Conn) Write(b []byte) (n int, err error) {
	return conn.Conn.Write(b)
}

func (conn *Conn) WriteChunks(c [][]byte) (n int, err error) {
	total := 0
	for i := 0; i < len(c); i++ {
		b, err := conn.Write(c[i])
		if err != nil {
			return 0, nil
		}

		b += total
	}

	return total, nil
}

func (conn *Conn) ReadBytes() ([]byte, error) {
	ret := make([]byte, 0)
	buf := make([]byte, BUF_SIZE)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}
		ret = append(ret, buf[:n]...)

		if n < BUF_SIZE {
			break
		}
	}

	return ret, nil
}

func (lConn *Conn) HandleHttp(p packet.HttpPacket) {
	ip, err := doh.Lookup(p.Domain)
	if err != nil {
		log.Debug("[HTTPS] Error looking up for domain: ", err)
	}
	log.Debug("[HTTPS] Found ip over HTTPS: ", ip)

	// Create connection to server
	rConn, err := Dial("tcp", ip+":80")
	if err != nil {
		log.Debug(err)
		return
	}
	defer rConn.Close()

	log.Debug("[HTTP] Connected to the server.")

	go rConn.Serve(lConn, "HTTP")

	_, err = rConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Debug("[HTTP] Error sending request to the server: ", err)
	}
	log.Debug("[HTTP] Sent a request to the server")

	go lConn.Serve(&rConn, "HTTP")
}

func (lConn *Conn) HandleHttps(p packet.HttpPacket) {
	ip, err := doh.Lookup(p.Domain)
	if err != nil {
		log.Debug("[HTTPS] Error looking up for domain: ", err)
	}
	log.Debug("[HTTPS] Found ip over HTTPS: ", ip)

	// Create a connection to the requested server
	rConn, err := Dial("tcp", ip+":443")
	if err != nil {
		log.Debug(err)
		return
	}
	defer rConn.Close()

	log.Debug("[HTTPS] Connected to the server.")

	_, err = lConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		log.Debug("[HTTPS] Error sending client hello: ", err)
	}
	log.Debug("[HTTPS] Sent 200 Connection Estabalished")

	// Read client hello
	clientHello, err := lConn.ReadBytes()
	if err != nil {
		log.Debug("[HTTPS] Error reading client hello: ", err)
		log.Debug("Closing connection: ", lConn.RemoteAddr())
	}

	log.Debug(lConn.RemoteAddr(), "[HTTPS] Client sent hello: ", len(clientHello), "bytes")

	// Generate a go routine that reads from the server
	go rConn.Serve(lConn, "HTTPS")

	pkt := packet.NewHttpsPacket(clientHello)

	chunks := pkt.SplitInChunks()

	if _, err := rConn.WriteChunks(chunks); err != nil {
		return
	}

	// Read from the client
	lConn.Serve(&rConn, "HTTPS")
}

func (from *Conn) Serve(to *Conn, proto string) {
	for {
		buf, err := from.ReadBytes()
		if err != nil {
			log.Debug("["+proto+"]"+" Error reading from ", from.RemoteAddr())
			log.Debug("["+proto+"]", err)
			log.Debug("[" + proto + "]" + " Exiting Serve() method. ")
			break
		}
		log.Debug(from.RemoteAddr(), " sent data: ", len(buf), "bytes")

		if _, err := to.Write(buf); err != nil {
			log.Debug("["+proto+"]"+"Error Writing to ", to.RemoteAddr())
			log.Debug("["+proto+"]", err)
			log.Debug("[" + proto + "]" + " Exiting Serve() method. ")
			break
		}
	}
}
