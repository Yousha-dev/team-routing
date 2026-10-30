//go:build abiprobe

package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
*/
import "C"

import "unsafe"

// Exercise the actual C entry point in tests, without shipping a test entry point.
func probeABICall(method string, body []byte, declaredLength uint64) []byte {
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var input unsafe.Pointer
	if len(body) > 0 {
		input = C.CBytes(body)
		defer C.free(input)
	}
	var response C.cliproxy_buffer
	C.cliproxyPluginCall(cMethod, (*C.uint8_t)(input), C.size_t(declaredLength), &response)
	defer C.cliproxyPluginFree(response.ptr, response.len)
	return C.GoBytes(response.ptr, C.int(response.len))
}
