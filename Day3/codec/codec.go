/**
 * @Author: Xuebi
 * @Description:
 * @File: codec
 * @Version: 1.0.0
 * @Date: 2021/5/7 16:36
 */

package codec

import "io"

type Header struct {
	ServiceMethod string //Service's name and Service method' name
	Seq uint64 //Seq is the request number, and is used to distinguish different requests.
	Error string //Information of error. For client, it can be nil, for sever, it is used to register error.
}

type Codec interface {
	io.Closer //Close() error
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header,interface{}) error
}

type NewCodecFunc func(io.ReadWriteCloser) Codec

type Type string

const (
	GobType Type = "application/gob"
	JsonType Type = "application/json"
)

var NewCodecFuncMap map[Type]NewCodecFunc

func init() {
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
}