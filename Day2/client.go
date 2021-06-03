/**
 * @Author: Xuebi
 * @Description:
 * @File: client.go
 * @Version: 1.0.0
 * @Date: 2021/5/11 15:20
 */

package XuebiRpc

import (
	"XuebiRpc/codec"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

// Call represents an active RPC.
type Call struct {
	Seq           uint64
	ServiceMethod string      // format "<service>.<method>"
	Args          interface{} // arguments to the function
	Reply         interface{} // reply from the function
	Error         error       // if error occurs, it will be set
	Done          chan *Call  // Strobes when call is complete.
}

func (call*Call) done() {
	call.Done <- call
}


// Client represents an RPC Client.
// There may be multiple outstanding Calls associated
// with a single Client, and a Client may be used by
// multiple goroutines simultaneously.

type Client struct {
	cc codec.Codec
	opt *Option
	sending sync.Mutex
	mu sync.Mutex
	header codec.Header
	seq uint64
	pending map[uint64]*Call
	closing bool
	shutdown bool
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

// Close the connection
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing

}

func (client *Client) registerCall(call *Call) (uint64, error){
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending,seq)
	return call
}

func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _,call := range client.pending {
		println("Pending---------------",call.Seq)
		call.Error = err
		call.done()
	}
}


func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}

		call := client.removeCall(h.Seq)
		switch {
		case call == nil :
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// error occurs, so terminateCalls pending calls
	client.terminateCalls(err)
}


func NewClient(conn net.Conn, opt*Option) (*Client,error) {
	/**
	 * @Author
	 * @Description 创建 Client 实例时:
					1. 首先需要完成一开始的协议交换，即发送 Option 信息给服务端。协商好消息的编解码方式之后，
					2. 再创建一个子协程调用 receive() 接收响应。
	 * @Date 17:23 2021/5/11
	 * @Param
	 * @return
	 **/
	f := codec.NewCodecFuncMap[opt.CodecType] //Get the constructor function
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(opt); err!=nil{
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec,opt*Option) *Client{
	client := &Client{
		seq : 1,
		cc: cc,
		opt: opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}


func parseOptions(opts ...*Option) (*Option, error) {
	/**
	 * @Author
	 * @Description 还需要实现 Dial 函数，便于用户传入服务端地址，创建 Client 实例。为了简化用户调用，通过 ...*Option 将 Option 实现为可选参数。
	 * @Date 17:24 2021/5/11
	 * @Param
	 * @return
	 **/

	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption,nil
	}
	if len(opts) != 1 {
		return nil,errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt,nil
}


func Dial(network, address string,opts ...*Option)(client *Client, err error){
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil,err
	}
	conn, err := net.Dial(network,address)
	if err != nil {
		return nil,err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	return NewClient(conn,opt)
}


func (client*Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()

	seq, err:= client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	if err := client.cc.Write(&client.header,call.Args);err!=nil {
		call := client.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go invokes the function asynchronously.
// It returns the Call structure representing the invocation.
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call{
	if done == nil {
		done = make(chan *Call,10)
	}else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args: args,
		Reply: reply,
		Done: done,
	}
	client.send(call)
	return call
}

func (client *Client) Call(serviceMethod string, args, reply interface{}) error {
	call := <-client.Go(serviceMethod, args, reply, make(chan *Call, 1)).Done
	return call.Error
}