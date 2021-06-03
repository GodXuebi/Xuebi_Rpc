/**
 * @Author: Xuebi
 * @Description: Use Gob format to decode and encode the information
 * @File: gob.go
 * @Version: 1.0.0
 * @Date: 2021/5/7 18:12
 */

package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder //Decode information from a bound IO
	enc  *gob.Encoder //Encode information from a bound IO
}

var _ Codec = (*GobCodec)(nil) //

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	/**
	 * @Author Xuebi
	 * @Description NewGobCodec constructor
	 * @Date 18:46 2021/5/7
	 * @Param io.ReadWriteCloser
	 * @return Codec
	 **/
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

func (c *GobCodec) ReadHeader(h *Header) error {
	return c.dec.Decode(h)
}

func (c *GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

func (c *GobCodec) Write(h *Header, body interface{}) (err error){
	defer func() {
		err = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()

	if err = c.enc.Encode(h); err!=nil {
		log.Println("rpc: gob error encoding header:", err)
		return
	}
	if err = c.enc.Encode(body); err !=nil {
		log.Println("rpc: gob error encoding body:", err)
		return
	}
	return
}

func (c *GobCodec) Close() error {
	return c.conn.Close()
}
