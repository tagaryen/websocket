package ws

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"websocket/logutil"
)

type WsSock struct {
	conn    *net.Conn
	url     string
	headers map[string]string
	buf     []byte
	len     int
	read    int
	write   int
	mask    byte
	maskKey [4]byte
	onRead  func([]byte)
}

type WsServerSock struct {
	runing    bool
	listenner *net.Listener
	onRead    func(*WsSock, []byte)
}

var log = logutil.GetDefaultLogger()

func (ws *WsSock) Close() {
	(*ws.conn).Close()
	ws.conn = nil
}

func (ws *WsSock) writeBuf(data []byte, off int, len int) error {
	if ws.len-(ws.write-ws.read) < len {
		return fmt.Errorf("ws socket buf shortage")
	} else if ws.len-ws.write < len {
		for i := 0; i < ws.write-ws.read; i++ {
			ws.buf[i] = ws.buf[ws.read+i]
		}
		ws.write = ws.write - ws.read
		ws.read = 0
	}
	if ws.mask == 1 {
		ws.maskKey[0] = data[off]
		ws.maskKey[1] = data[off+1]
		ws.maskKey[2] = data[off+2]
		ws.maskKey[3] = data[off+3]
		off += 4
		for i := 0; i < len; i++ {
			ws.buf[ws.write] = data[off+i] ^ ws.maskKey[i%4]
			ws.write++
		}
	} else {
		byteCopy(ws.buf, ws.write, data, off, len)
		ws.write += len
	}
	return nil
}

func (ws *WsSock) readIn() [][]byte {
	content := make([]byte, 1024*4)
	read, err := (*ws.conn).Read(content)
	if err != nil {
		log.Error("ws read data error %s", err.Error())
		ws.Close()
		return nil
	}
	off := 0
	ret := make([][]byte, 8)
	retOff := 0
	for {
		if off >= read {
			break
		}
		if ws.len == 0 {
			off++
			mask := content[off] >> 7
			payloadLen := int(content[off] & 0x7F)
			off++
			if payloadLen == 126 {
				payloadLen = (int(content[off]) << 8) + int(content[off+1])
				off += 2
			}
			if payloadLen == 127 {
				off += 4
				payloadLen = (int(content[off]) << 24) + (int(content[off+1]) << 16) + (int(content[off+2]) << 8) + int(content[off+3])
				off += 4
			}
			if payloadLen > read-off {
				ws.len, ws.read, ws.write, ws.mask = payloadLen, 0, 0, mask
				ws.writeBuf(content, off, read-off)
				break
			} else {
				ret[retOff] = make([]byte, payloadLen)
				if mask == 1 {
					maskingKey := content[off : off+4]
					off += 4
					for i := 0; i < payloadLen; i++ {
						ret[retOff][i] = content[off+i] ^ maskingKey[i%4]
					}
				} else {
					byteCopy(ret[retOff], 0, content, off, payloadLen)
				}
				off += payloadLen
				retOff++
			}
		} else {
			remain := ws.len - (ws.write + ws.read)
			if remain <= read-off {
				ret[retOff] = make([]byte, ws.len)
				retOOff := ws.write - ws.read
				byteCopy(ret[retOff], 0, ws.buf, ws.read, retOOff)
				if ws.mask == 1 {
					for i := 0; i < remain; i++ {
						ret[retOff][i+retOOff] = content[off+i] ^ ws.maskKey[i%4]
					}
				} else {
					byteCopy(ret[retOff], 0, content, off, remain)
				}
				off += remain
				retOff++
				ws.read, ws.write, ws.len, ws.mask = 0, 0, 0, 0
			} else {
				ws.writeBuf(content, off, read-off)
				break
			}
		}
	}
	return ret[:retOff]
}

func (ws *WsSock) Send(data []byte) error {
	length := len(data)
	resBytes := []byte{byte(129)}
	if length >= 65536 {
		resBytes = append(resBytes, byte(127))
		resBytes = append(resBytes, ' ')
		resBytes = append(resBytes, ' ')
		resBytes = append(resBytes, ' ')
		resBytes = append(resBytes, ' ')
		b4 := make([]byte, 4)
		b4[0] = uint8(length >> 24)
		b4[1] = uint8(length >> 16)
		b4[2] = uint8(length >> 8)
		b4[3] = uint8(length)
		resBytes = append(resBytes, b4...)
	} else if length >= 126 {
		resBytes = append(resBytes, byte(126))
		b2 := make([]byte, 2)
		b2[0] = uint8(length >> 8)
		b2[1] = uint8(length)
		resBytes = append(resBytes, b2...)
	} else {
		resBytes = append(resBytes, byte(length))
	}
	resBytes = append(resBytes, data...)
	(*ws.conn).Write(resBytes)
	return nil
}

func (ws *WsSock) SetOnRead(onRead func([]byte)) {
	ws.onRead = onRead
}

func NewServer() *WsServerSock {
	return &WsServerSock{}
}

func (server *WsServerSock) StartServer(host string, port int) {
	addr := fmt.Sprintf("%s:%d", host, port)
	log.Info("listenning at %s", addr)
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		log.Error("%s", err.Error())
	}
	server.listenner = &listener
	server.runing = true
	for {
		if !server.runing {
			listener.Close()
			break
		}
		conn, err := listener.Accept()
		if err != nil {
			log.Error("%s", err.Error())
			conn.Close()
			continue
		}
		go server.doAccept(&conn)
	}
}

func (server *WsServerSock) StopServer() {
	server.runing = false
}

func (server *WsServerSock) SetOnRead(onRead func(*WsSock, []byte)) {
	server.onRead = onRead
}

func (server *WsServerSock) doAccept(conn *net.Conn) {
	var buf = make([]byte, 1024*4)
	off, err := (*conn).Read(buf)
	if err != nil {
		log.Error("%s", err.Error())
		(*conn).Close()
		return
	}
	if off < 64 {
		log.Warn("receive data shortage")
		(*conn).Close()
		return
	}
	ws := parseWsRequest(string(buf[:off]), conn)
	for {
		if server.runing {
			msgs := ws.readIn()
			if msgs == nil {
				log.Error("parse ws message failed.")
				ws.Close()
			}
			for _, msg := range msgs {
				server.onRead(ws, msg)
			}
		} else {
			break
		}
	}
}

func getUpgradeWsResponse(secWebSocketAccept string) []byte {
	response := "HTTP/1.1 101 Switching Protocols\r\n"
	response = response + "Sec-WebSocket-Accept: " + string(secWebSocketAccept) + "\r\n"
	response = response + "Connection: Upgrade\r\n"
	response = response + "Upgrade: websocket\r\n\r\n"

	responseBytes := []byte(response)
	return responseBytes
}

func wsHandshake(wsreq *WsSock) bool {

	connection := wsreq.headers["Connection"]
	if connection != "Upgrade" {
		log.Warn("Connection must be 'Upgrade' but got '%s'", connection)
		return false
	}
	upgrade := wsreq.headers["Upgrade"]
	if upgrade != "websocket" {
		log.Warn("Upgrade must be 'websocket' but got '%s'", upgrade)
		return false
	}
	secWebsocketKey := wsreq.headers["Sec-WebSocket-Key"]
	if secWebsocketKey == "" {
		log.Warn("Sec-WebSocket-Key not found")
		return false
	}

	h := sha1.New()
	io.WriteString(h, secWebsocketKey+"258EAFA5-E914-47DA-95CA-C5AB0DC85B11") //magic number
	accept := make([]byte, 28)
	base64.StdEncoding.Encode(accept, h.Sum(nil))
	response := getUpgradeWsResponse(string(accept))
	if _, err := (*wsreq.conn).Write(response); err != nil {
		log.Error("upgrade ws protocol failed, %s", err.Error())
		return false
	}
	return true
}

func parseWsRequest(content string, conn *net.Conn) *WsSock {
	wsreq := &WsSock{}
	if content[:3] != "GET" {
		log.Warn("invalid ws message %s", content)
		(*conn).Close()
		return nil
	}
	contents := strings.Split(content, "\r\n")
	seps := strings.Split(contents[0], " ")
	if len(seps) != 3 {
		log.Warn("invalid ws message %s", content)
		(*conn).Close()
		return nil
	}
	wsreq.url = seps[1]
	wsreq.headers = make(map[string]string, 64)
	for i := 1; i < len(contents); i++ {
		if contents[i] == "" {
			break
		}
		headkv := strings.Split(contents[i], ": ")
		if len(headkv) != 2 {
			log.Warn("invalid ws head %s", contents[i])
			(*conn).Close()
			return nil
		}
		wsreq.headers[headkv[0]] = headkv[1]
	}

	wsreq.buf = make([]byte, 4*1024)
	wsreq.conn = conn

	if !wsHandshake(wsreq) {
		(*conn).Close()
		return nil
	}
	return wsreq
}

func byteCopy(dst []byte, dstOff int, src []byte, srcOff int, len int) {
	for i := 0; i < len; i++ {
		dst[dstOff+i] = src[srcOff+i]
	}
}
