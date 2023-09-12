package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/v2fly/v2ray-core/v5/main/commands/base"
	"net"
	"os"
)

func main() {
	tcpConn, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: []byte{127, 0, 0, 1}, Port: 1234})
	if err != nil {
		base.Fatalf("Failed to dial tcp: %s", err)
	}
	pool := x509.NewCertPool()
	file, err := os.ReadFile("H:\\certs\\second.crt")
	if file != nil {
		pool.AppendCertsFromPEM(file)
	}
	file, err = os.ReadFile("H:\\certs\\root.crt")
	if file != nil {
		pool.AppendCertsFromPEM(file)
	}
	//file, err = os.ReadFile("H:\\certs\\client.crt")
	if file != nil {
		pool.AppendCertsFromPEM(file)
	}
	cert, err := tls.LoadX509KeyPair("H:\\certs\\client.crt", "H:\\certs\\client.key")

	tlsConn := tls.Client(tcpConn, &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"http/1.1"},
		MaxVersion:         tls.VersionTLS12,
		MinVersion:         tls.VersionTLS12,
		RootCAs:            pool,
		Certificates:       []tls.Certificate{cert},
		// Do not release tool before v5's refactor
		//VerifyPeerCertificate: showCert(),
	})

	err = tlsConn.Handshake()
	if err != nil {
		fmt.Println("Handshake failure: ", err)
	} else {
		fmt.Println("Handshake succeeded")
		printCertificates(tlsConn.ConnectionState().PeerCertificates)
	}
	tlsConn.Close()
}
func printCertificates(certs []*x509.Certificate) {
	for _, cert := range certs {
		if len(cert.DNSNames) == 0 {
			continue
		}
		fmt.Println("Allowed domains: ", cert.DNSNames)
	}
}
