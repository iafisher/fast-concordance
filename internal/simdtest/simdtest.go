package simdtest

// #cgo CFLAGS: -mavx2
// #include <immintrin.h>
// #include <stdio.h>
// #include <string.h>
// 
// ssize_t simd_str_search(const char* text, const char* keyword) {
// 	unsigned char keyword_padded[32] = {0};
// 	unsigned char keyword_mask[32] = {0};
// 	for (size_t i = 0; keyword[i] != '\0'; i++) {
// 		keyword_padded[i] = keyword[i];
// 		keyword_mask[i] = 1;
// 	}
// 	__m256i keywordv = _mm256_loadu_si256((__m256i*)keyword_padded);
// 	__m256i mask = _mm256_loadu_si256((__m256i*)keyword_mask);
// 
// 	size_t n = strlen(text);
// 	if (n < 32) {
// 		puts("error: text too short");
// 		return -1;
// 	}
// 
// 	for (size_t i = 0; i <= n - 32; i++) {
// 		__m256i textv = _mm256_loadu_si256((__m256i*)(text + i));
// 		__m256i r1 = _mm256_xor_si256(textv, keywordv);
// 		__m256i r2 = _mm256_and_si256(r1, mask);
// 		if (_mm256_testz_si256(r2, r2)) {
// 			return i;
// 		}
// 	}
// 
// 	return -1;
// }
import "C"

import (
	"unsafe"
)

func Search(text string, keyword string) int {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cKeyword := C.CString(keyword)
	defer C.free(unsafe.Pointer(cKeyword))

	return int(C.simd_str_search(cText, cKeyword))
}
