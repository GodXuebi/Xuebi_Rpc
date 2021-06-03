/**
 * @Author: Xuebi
 * @Description:
 * @File: server.go
 * @Version: 1.0.0
 * @Date: 2021/5/8 13:44
 */

package XuebiRpc

import (
	"XuebiRpc/codec"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int
	CodecType   codec.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

// DefaultServer is the default instance of *Server.
type Server struct {
	serviceMap sync.Map
}

func NewServer() *Server {
	return &Server{}
}

//首先定义了结构体 Server，没有任何的成员字段。
//实现了 Accept 方式，net.Listener 作为参数，for 循环等待 socket 连接建立，并开启子协程处理，处理过程交给了 ServerConn 方法。
//DefaultServer 是一个默认的 Server 实例，主要为了用户使用方便。
var DefaultServer = NewServer()

func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return errors.New("rpc: service already defined: " + s.name)
	}
	return nil
}

func (server *Server) Accept(lis net.Listener) {
	/**
	 * @Author  Xuebi
	 * @Description Accept accepts connections on the listener and serves requests
					for each incoming connection.
		 * @Date 13:59 2021/5/8
	 * @Param
	 * @return
	 **/
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		go server.ServeConn(conn)

	}
}

func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}

func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed: " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service " + serviceName)
		return
	}
	svc = svci.(*service)
	mtype = svc.methods[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method " + methodName)
	}
	return

}

//conn io.ReadWriteCloser == conn net.Conn
func (server *Server) ServeConn(conn net.Conn) {
	/**
	 * @Author
	 * @Description
	| Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
	| <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|
	 * @Date 14:07 2021/5/8
	 * @Param
	 * @return
	 **/
	defer func() { _ = conn.Close() }()
	var opt Option
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number %x", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
		return
	}
	server.ServeCodec(f(conn))
}

var invalidRequest = struct{}{}

func (server *Server) ServeCodec(cc codec.Codec) {
	/**
	 * @Author
	 * @Description
		serveCodec 的过程非常简单。主要包含三个阶段
		 读取请求 readRequest
		 处理请求 handleRequest
		 回复请求 sendResponse
	  * @Date 14:17 2021/5/8
	 * @Param
	 * @return
	 **/
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	for {
		req, err := server.readRequest(cc)
		//log.Println("server: ServeCodec",req.h)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg)
	}
	wg.Wait()
	_ = cc.Close()

}

// request stores all information of a call
type request struct {
	h      *codec.Header // header of request
	argv   reflect.Value // argv and replyv of request
	replyv reflect.Value
	mtype  *methodType
	svc    *service
}

func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc,req.mtype,err = server.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()

	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read body err:", err)
		return req, err
	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	//log.Println("server: sendingResponse",h,body)
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	err := req.svc.call(req.mtype,req.argv,req.replyv)
	if err != nil {
		req.h.Error = err.Error()
		server.sendResponse(cc, req.h, invalidRequest, sending)
		return
	}
	server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
}
