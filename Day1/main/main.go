/**
 * @Author: Xuebi
 * @Description:
 * @File: main.go
 * @Version: 1.0.0
 * @Date: 2021/5/8 15:00
 */

package main

import (
	"encoding/json"
	"fmt"
	"XuebiRpc"
	"XuebiRpc/codec"
	"log"
	"net"
	"time"
)

func startServer(addr chan string) {
	l, err := net.Listen("tcp",":0")
	if err != nil {
		log.Fatal("network error:", err)
	}
	log.Println("start rpc server on", l.Addr())
	addr <- l.Addr().String()
	XuebiRpc.Accept(l)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	// in fact, following code is like a simple geerpc client
	conn, _ := net.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)
	// send options
	_ = json.NewEncoder(conn).Encode(XuebiRpc.DefaultOption)
	cc := codec.NewGobCodec(conn)
	// send request & receive response
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("XuebiRpc req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)
	}
}