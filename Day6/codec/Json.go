/**
 * @Author: Xuebi
 * @Description: Use Json format to decode and encode the information
 * @File: Json.go
 * @Version: 1.0.0
 * @Date: 2021/5/7 18:49
 */

package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

type JsonCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *json.Decoder //Decode information from a bound IO
	enc  *json.Encoder //Encode information from a bound IO

}

var _ Codec = (*JsonCodec)(nil)

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	/**
	 * @Author Xuebi
	 * @Description NewJsonCodec constructor
	 * @Date 18:46 2021/5/7
	 * @Param io.ReadWriteCloser
	 * @return Codec
	 **/
	buf := bufio.NewWriter(conn)
	return &JsonCodec{
		conn: conn,
		buf:  buf,
		dec:  json.NewDecoder(conn),
		enc:  json.NewEncoder(buf),
	}
}
func (c *JsonCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *JsonCodec) ReadBody(b interface{}) error {
	return c.dec.Decode(b)
}

func (c *JsonCodec) Write(h *Header, body interface{}) (err error) {
	defer func() {
		err = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(h); err != nil {
		log.Println("rpc: json error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err != nil {
		log.Println("rpc: json error encoding body:", err)
		return
	}
	return
}

func (c *JsonCodec) Close() error {
	return c.conn.Close()
}
