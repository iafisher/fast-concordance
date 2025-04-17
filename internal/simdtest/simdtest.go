package simdtest

// TODO: make this amd64 only
// TODO: move C code to its own file (but then can I use GoString functions?)

// #cgo CFLAGS: -mavx2
// #include <immintrin.h>
// #include <stdio.h>
// #include <string.h>
//
// ssize_t simd_str_search(_GoString_ text, _GoString_ keyword, size_t offset) {
//	size_t text_len = _GoStringLen(text);
//	size_t keyword_len = _GoStringLen(keyword);
// 	if (text_len < 32 || keyword_len > 32) {
//		return -1;
//	}
// 	unsigned char keyword_padded[32] = {0};
// 	unsigned char keyword_mask[32] = {0};
//	const char* keyword_p = _GoStringPtr(keyword);
// 	for (size_t i = 0; i < keyword_len; i++) {
// 		keyword_padded[i] = keyword_p[i];
// 		keyword_mask[i] = 1;
// 	}
// 	__m256i keywordv = _mm256_loadu_si256((__m256i*)keyword_padded);
// 	__m256i mask = _mm256_loadu_si256((__m256i*)keyword_mask);
//
//	const char* text_p = _GoStringPtr(text);
// 	for (size_t i = offset; i <= text_len - 32; i++) {
// 		__m256i textv = _mm256_loadu_si256((__m256i*)(text_p + i));
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

func Search(text string, keyword string, offset int) int {
	return int(C.simd_str_search(text, keyword, C.size_t(offset)))
}
