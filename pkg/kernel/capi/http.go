//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unsafe"
)

func httpSend(_ unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	if nargs < 1 {
		setStringResult(results, false, "invalid HTTP request")
		return
	}
	request := argValue(args, 0)
	method := valueString(recordField(request, "method"))
	url := valueString(recordField(request, "url"))
	limit := int64(C.rc_u64(recordField(request, "max-response-bytes")))
	if limit <= 0 || limit > 48<<20 {
		limit = 48 << 20
	}
	body := valueBytes(recordField(request, "body"))
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	headers := recordField(request, "headers")
	for index := 0; index < int(C.rc_list_len(headers)); index++ {
		header := C.rc_list_get(headers, C.size_t(index))
		req.Header.Add(valueString(recordField(header, "name")), valueString(recordField(header, "value")))
	}
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	if int64(len(payload)) > limit {
		setStringResult(results, false, "response exceeds max-response-bytes")
		return
	}
	value := newValue()
	C.rc_alloc_record(value, 4)
	C.rc_set_u16(C.rc_record_item(value, 0, cString("status"), 6), C.uint16_t(response.StatusCode))
	*C.rc_record_item(value, 1, cString("final-url"), 9) = *newStringValue(response.Request.URL.String())
	headerList := newValue()
	C.rc_alloc_list(headerList, C.size_t(len(response.Header)))
	index := 0
	for name, values := range response.Header {
		for _, entry := range values {
			header := C.rc_list_item(headerList, C.size_t(index))
			C.rc_alloc_record(header, 2)
			*C.rc_record_item(header, 0, cString("name"), 4) = *newStringValue(name)
			*C.rc_record_item(header, 1, cString("value"), 5) = *newStringValue(entry)
			index++
		}
	}
	C.rc_of_list(headerList, C.size_t(index))
	*C.rc_record_item(value, 2, cString("headers"), 7) = *headerList
	*C.rc_record_item(value, 3, cString("body"), 4) = *newBytesValue(payload)
	C.rc_set_result(results, true, value)
}

var _ = fmt.Sprintf
var _ = strings.TrimSpace
