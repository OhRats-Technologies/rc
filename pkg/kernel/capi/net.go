//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const resourceConnection uint32 = 8

type netConnection struct {
	mu             sync.Mutex
	conn           net.Conn
	idleTimeout    time.Duration
	maxLifetime    time.Duration
	created        time.Time
	closedForWrite bool
}

var (
	netHosts    sync.Map
	netSequence atomic.Uint64
)

func newNetHost(endpoints []string) unsafe.Pointer {
	allowed := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		allowed[endpoint] = true
	}
	id := netSequence.Add(1)
	netHosts.Store(id, allowed)
	slot := C.malloc(8)
	*(*C.uint64_t)(slot) = C.uint64_t(id)
	return slot
}

func releaseNetHost(slot unsafe.Pointer) {
	if slot == nil {
		return
	}
	id := uint64(*(*C.uint64_t)(slot))
	netHosts.Delete(id)
	C.free(slot)
}

func netAllowed(data unsafe.Pointer, host string, port uint16) bool {
	if data == nil {
		return false
	}
	id := uint64(*(*C.uint64_t)(data))
	value, ok := netHosts.Load(id)
	if !ok {
		return false
	}
	return value.(map[string]bool)[host+":"+strconv.Itoa(int(port))]
}

func netConnect(ctx unsafe.Pointer, data unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 2 {
		setStringResult(results, false, "invalid connect request")
		return
	}
	endpoint := argValue(args, 0)
	host := valueString(recordField(endpoint, "host"))
	port := uint16(C.rc_u16(recordField(endpoint, "port")))
	if !netAllowed(data, host, port) {
		setStringResult(results, false, "endpoint is not allowed")
		return
	}
	limits := argValue(args, 1)
	connectTimeout := time.Duration(C.rc_u64(recordField(limits, "connect-timeout-ms"))) * time.Millisecond
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}
	idleTimeout := time.Duration(C.rc_u64(recordField(limits, "idle-timeout-ms"))) * time.Millisecond
	maxLifetime := time.Duration(C.rc_u64(recordField(limits, "max-lifetime-ms"))) * time.Millisecond
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(int(port))), connectTimeout)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	object := &netConnection{conn: connection, idleTimeout: idleTimeout, maxLifetime: maxLifetime, created: time.Now()}
	resource, err := hostResource(ctx, addResourceObject(object), resourceConnection)
	if err != nil {
		_ = connection.Close()
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_resource(payload, resource)
	C.rc_set_result(results, true, payload)
}

func netWrite(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	connection, ok := selfConnection(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "unknown connection")
		return
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.conn == nil || connection.closedForWrite {
		setStringResult(results, false, "connection is closed")
		return
	}
	connection.conn.SetWriteDeadline(time.Now().Add(connection.idleTimeout))
	n, err := connection.conn.Write(valueBytes(argValue(args, 1)))
	if err != nil && n == 0 {
		if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
			C.rc_set_result(results, true, newVariantValue("would-block", nil))
			return
		}
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_u64(payload, C.uint64_t(n))
	C.rc_set_result(results, true, newVariantValue("accepted", payload))
}

func netRead(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	connection, ok := selfConnection(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "unknown connection")
		return
	}
	maxBytes := int(C.rc_u64(argValue(args, 1)))
	if maxBytes < 1 {
		maxBytes = 1
	}
	if maxBytes > 1<<20 {
		maxBytes = 1 << 20
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.conn == nil {
		setStringResult(results, false, "connection is closed")
		return
	}
	if connection.maxLifetime > 0 && time.Since(connection.created) > connection.maxLifetime {
		setStringResult(results, false, "connection lifetime exceeded")
		return
	}
	connection.conn.SetReadDeadline(time.Now().Add(connection.idleTimeout))
	buffer := make([]byte, maxBytes)
	n, err := connection.conn.Read(buffer)
	if n > 0 {
		payload := newValue()
		setBytesValue(payload, buffer[:n])
		C.rc_set_result(results, true, newVariantValue("data", payload))
		return
	}
	if err == io.EOF {
		C.rc_set_result(results, true, newVariantValue("eof", nil))
		return
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		C.rc_set_result(results, true, newVariantValue("would-block", nil))
		return
	}
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	C.rc_set_result(results, true, newVariantValue("would-block", nil))
}

func netCloseWrite(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	connection, ok := selfConnection(ctx, args)
	if !ok {
		setStringResult(results, false, "unknown connection")
		return
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if tcp, ok := connection.conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	connection.closedForWrite = true
	C.rc_set_result(results, true, nil)
}

func netClose(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, _ *C.wasmtime_component_val_t, _ C.size_t) {
	if connection, ok := selfConnection(ctx, args); ok {
		connection.mu.Lock()
		if connection.conn != nil {
			_ = connection.conn.Close()
			connection.conn = nil
		}
		connection.mu.Unlock()
	}
}

func selfConnection(ctx unsafe.Pointer, args *C.wasmtime_component_val_t) (*netConnection, bool) {
	rep, ok := selfRep(ctx, args)
	if !ok {
		return nil, false
	}
	connection, ok := lookupResource(rep).(*netConnection)
	return connection, ok
}

func releaseConnection(connection *netConnection) {
	if connection == nil {
		return
	}
	connection.mu.Lock()
	defer connection.mu.Unlock()
	if connection.conn != nil {
		_ = connection.conn.Close()
		connection.conn = nil
	}
}
