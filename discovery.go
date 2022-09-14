package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/net/ipv6"
)

func find_link_local_address() (ip string, i net.Interface, err error) {
	//find an interface that looks like ethernet and has an ipv6 link local address
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", i, err
	}

	for _, i := range ifaces {
		name := strings.ToLower(i.Name)
		//guess if an interface is ethernet by looking at its name
		//    windows has "Ethernet #", linux has "eth#" or "enp###"
		if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "enp") {
			addrs, _ := i.Addrs()
			for _, a := range addrs {
				ip := net.ParseIP(strings.Split(a.String(), "/")[0])

				//if its link local and ipv6 then use it
				if ip.IsLinkLocalUnicast() && strings.Contains(ip.String(), ":") {
					return fmt.Sprintf("%s%%%d", ip.String(), i.Index), i, nil
				}
			}
		}
	}
	return "", i, fmt.Errorf("ethernet interface not found")
}

func bind_multicast(address string, port int, i net.Interface) (conn *ipv6.PacketConn) {
	//bind to address:port and join the link-local multicast group

	c, err := net.ListenPacket("udp6", fmt.Sprintf("[%s]:%d", address, port))
	if err != nil {
		fmt.Println(err.Error())
		terminate()
	}

	p := ipv6.NewPacketConn(c)
	p.SetMulticastLoopback(false)

	multicast := &net.UDPAddr{IP: net.ParseIP(MULTICAST)}

	p.JoinGroup(&i, multicast)

	return p

}

func responder(local string, i net.Interface) {
	//to recieve a multicast we need to bind to the multicast address
	//to send a multicast we need to bind to the link local address
	r := bind_multicast(MULTICAST, RECV_PORT, i)
	s := bind_multicast(local, SEND_PORT, i)

	data := make([]byte, 64)

	//multicast to our peer thats currently in discover mode
	destination := &net.UDPAddr{IP: net.ParseIP(MULTICAST), Port: RECV_PORT + 1}

	//strip the zone identifier (peer will add their own)
	local_ip := strings.Split(local, "%")[0]

	for {
		r.ReadFrom(data) //blocks until a peer makes a connection
		s.WriteTo([]byte(local_ip), nil, destination)
	}
}

func discover(local string, i net.Interface) (remote string) {
	//use different ports to the responder so we can recieve and send concurrently
	r := bind_multicast(MULTICAST, RECV_PORT+1, i)
	s := bind_multicast(local, SEND_PORT+1, i)
	defer r.Close()
	defer s.Close()

	//multicast to our peer who's currently in responder mode
	destination := &net.UDPAddr{IP: net.ParseIP(MULTICAST), Port: RECV_PORT}

	data := make([]byte, 64)

	for {
		s.WriteTo([]byte(REQUEST), nil, destination)
		//keep sending requests every 100 milliseconds until we get a response
		r.SetReadDeadline(time.Now().Add(time.Millisecond * 100))
		n, _, _, _ := r.ReadFrom(data)
		if n != 0 {
			data = data[:n]
			break
		}
	}

	//add our interfaces zone identifier since link-local addresses are routable over any interface
	remote = fmt.Sprintf("%s%%%d", string(data), i.Index)

	return remote
}
