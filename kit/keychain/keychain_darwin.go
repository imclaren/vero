//go:build darwin && cgo

package keychain

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

static CFMutableDictionaryRef query(const char *service, const char *account) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef s = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef a = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(q, kSecAttrService, s);
	CFDictionarySetValue(q, kSecAttrAccount, a);
	CFRelease(s);
	CFRelease(a);
	return q;
}

static OSStatus kc_get(const char *service, const char *account, CFDataRef *out) {
	CFMutableDictionaryRef q = query(service, account);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	OSStatus st = SecItemCopyMatching(q, (CFTypeRef *)out);
	CFRelease(q);
	return st;
}

static OSStatus kc_set(const char *service, const char *account, const void *data, long n) {
	CFDataRef d = CFDataCreate(NULL, data, n);
	CFMutableDictionaryRef q = query(service, account);
	CFMutableDictionaryRef change = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(change, kSecValueData, d);
	OSStatus st = SecItemUpdate(q, change);
	if (st == errSecItemNotFound) {
		CFDictionarySetValue(q, kSecValueData, d);
		st = SecItemAdd(q, NULL);
	}
	CFRelease(change);
	CFRelease(q);
	CFRelease(d);
	return st;
}

static OSStatus kc_del(const char *service, const char *account) {
	CFMutableDictionaryRef q = query(service, account);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return st;
}

static char *kc_message(OSStatus st) {
	CFStringRef s = SecCopyErrorMessageString(st, NULL);
	if (s == NULL) return NULL;
	CFIndex n = CFStringGetMaximumSizeForEncoding(CFStringGetLength(s), kCFStringEncodingUTF8) + 1;
	char *buf = malloc(n);
	if (!CFStringGetCString(s, buf, n, kCFStringEncodingUTF8)) buf[0] = 0;
	CFRelease(s);
	return buf;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const available = true

func get(service, account string) ([]byte, error) {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	var out C.CFDataRef
	if st := C.kc_get(s, a, &out); st != C.errSecSuccess {
		return nil, failure(st)
	}
	defer C.CFRelease(C.CFTypeRef(out))
	return C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(out)), C.int(C.CFDataGetLength(out))), nil
}

func set(service, account string, data []byte) error {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	p := C.CBytes(data)
	defer C.free(p)
	if st := C.kc_set(s, a, p, C.long(len(data))); st != C.errSecSuccess {
		return failure(st)
	}
	return nil
}

func del(service, account string) error {
	s, a := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(s))
	defer C.free(unsafe.Pointer(a))
	if st := C.kc_del(s, a); st != C.errSecSuccess {
		return failure(st)
	}
	return nil
}

func failure(st C.OSStatus) error {
	if st == C.errSecItemNotFound {
		return ErrNotFound
	}
	msg := "OSStatus"
	if m := C.kc_message(st); m != nil {
		msg = C.GoString(m)
		C.free(unsafe.Pointer(m))
	}
	return fmt.Errorf("keychain: %s (%d)", msg, int(st))
}
