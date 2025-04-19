package simdsearch

// TODO: make this amd64 only
// TODO: move C code to its own file (but then can I use GoString functions?)

// #cgo CFLAGS: -mavx2
// #include <immintrin.h>
// #include <stdio.h>
// #include <string.h>
//
// void print_avx_vector(const char* label, __m256i x) {
// 	int x1 = _mm256_extract_epi32(x, 0);
// 	int x2 = _mm256_extract_epi32(x, 1);
// 	int x3 = _mm256_extract_epi32(x, 2);
// 	int x4 = _mm256_extract_epi32(x, 3);
// 	int x5 = _mm256_extract_epi32(x, 4);
// 	int x6 = _mm256_extract_epi32(x, 5);
// 	int x7 = _mm256_extract_epi32(x, 6);
// 	int x8 = _mm256_extract_epi32(x, 7);
// 	printf("%s: %d %d %d %d %d %d %d %d\n", label, x1, x2, x3, x4, x5, x6, x7, x8);
// }
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
// 		keyword_mask[i] = 0xFF;
// 	}
// 	__m256i keywordv = _mm256_loadu_si256((__m256i*)keyword_padded);
// 	__m256i mask = _mm256_loadu_si256((__m256i*)keyword_mask);
//	// print_avx_vector("keywordv", keywordv);
//	// print_avx_vector("mask", mask);
//
//	const char* text_p = _GoStringPtr(text);
// 	for (size_t i = offset; i <= text_len - 32; i++) {
// 		// printf("index: %lu\n", i);
// 		__m256i textv = _mm256_loadu_si256((__m256i*)(text_p + i));
//		// print_avx_vector("textv", textv);
// 		__m256i r1 = _mm256_xor_si256(textv, keywordv);
//		// print_avx_vector("r1", r1);
// 		__m256i r2 = _mm256_and_si256(r1, mask);
//		// print_avx_vector("r2", r2);
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
