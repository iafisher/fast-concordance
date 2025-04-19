#include <dirent.h>
#include <fcntl.h>
#include <immintrin.h>
#include <pthread.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <time.h>
#include <unistd.h>

void print_avx_vector(const char* label, __m256i x) {
	int x1 = _mm256_extract_epi32(x, 0);
	int x2 = _mm256_extract_epi32(x, 1);
	int x3 = _mm256_extract_epi32(x, 2);
	int x4 = _mm256_extract_epi32(x, 3);
	int x5 = _mm256_extract_epi32(x, 4);
	int x6 = _mm256_extract_epi32(x, 5);
	int x7 = _mm256_extract_epi32(x, 6);
	int x8 = _mm256_extract_epi32(x, 7);
	printf("%s: %d %d %d %d %d %d %d %d\n", label, x1, x2, x3, x4, x5, x6, x7, x8);
}

typedef ssize_t (*str_search_t)(const char*, const char*, size_t);

struct text {
    const char* filename;
    char* contents;
    size_t length;
};

struct corpus {
    size_t length, capacity;
    struct text* texts;
};

void bail(const char* msg) {
    perror(msg);
    exit(EXIT_FAILURE);
}

void read_all(int fd, char* buf, size_t real_capacity) {
    // save room for null terminator
    size_t capacity = real_capacity - 1;
    size_t offset = 0;
    while (1) {
        ssize_t nread = read(fd, buf + offset, capacity - offset);
        if (nread < 0) {
            bail("read failed");
        } else if (nread == 0) {
            break;
        }
        offset += nread;
    }
    buf[real_capacity - 1] = '\0';
}

struct corpus load_all_texts(const char* directory) {
    size_t capacity = 2000;
    struct text* texts = malloc((sizeof *texts) * capacity);
    if (texts == NULL) {
        bail("malloc failed");
    }

    int dirfd = open(directory, O_RDONLY | O_DIRECTORY);
    DIR* d = fdopendir(dirfd);
    if (d == NULL) {
        bail("opendir failed");
    }

    struct dirent* ent;
    size_t i = 0;
    while ((ent = readdir(d)) != NULL) {
        if (i == capacity) {
            bail("ran out of room for texts");
        }

        if (strcmp(ent->d_name, ".") == 0 || strcmp(ent->d_name, "..") == 0 || ent->d_type != DT_DIR) {
            continue;
        }

        texts[i].filename = strdup(ent->d_name);
        struct stat statbuf;
        int subdirfd = openat(dirfd, texts[i].filename, O_RDONLY | O_DIRECTORY);
        if (subdirfd < 0) { bail("openat failed"); }

        int fd = openat(subdirfd, "merged.txt", O_RDONLY);
        if (fd < 0) { bail("openat failed"); }
        int r = fstat(fd, &statbuf);
        if (r < 0) { bail("fstatat failed"); }

        size_t capacity = statbuf.st_size + 1;
        texts[i].contents = malloc(capacity);
        if (texts[i].contents == NULL) { bail("malloc failed"); }

        read_all(fd, texts[i].contents, capacity);
        texts[i].length = capacity - 1;

        close(subdirfd);
        close(fd);
        i++;
    }

    closedir(d);
    close(dirfd);
    return (struct corpus){ .length = i, .capacity = capacity, .texts = texts };
}

int is_letter(char c) {
    return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z');
}

struct thread_context {
    str_search_t search_f;
    struct corpus corpus;
    size_t start, end;
    const char* keyword;
};

int search_one(str_search_t search_f, struct text text, const char* keyword, int print_matches) {
    int count = 0;
    size_t offset = 0;
    size_t keyword_len = strlen(keyword);
    while (1) {
        ssize_t index = search_f(text.contents, keyword, offset);
        if (index == -1) {
            break;
        }
        offset = index + 1;

        if (index > 0 && is_letter(text.contents[index - 1])) {
            continue;
        }

        if (index + keyword_len < text.length && is_letter(text.contents[index + keyword_len])) {
            continue;
        }

        if (print_matches) {
            int match_context = 20;
            size_t match_start = (index >= match_context) ? (index - match_context) : 0;
            int full_match_len = (2 * match_context) + keyword_len;
            int match_len = (match_start + full_match_len <= text.length) ? full_match_len : (text.length - match_start);
            printf("%s: ", text.filename);
            fwrite(text.contents + match_start, match_len, 1, stdout);
            printf("\n");
        }

        count += 1;
    }
    return count;
}

void* search_thread(void* arg) {
    struct thread_context* ctx = (struct thread_context*)arg;
    int count = 0;

    for (size_t i = ctx->start; i < ctx->end; i++) {
        count += search_one(ctx->search_f, ctx->corpus.texts[i], ctx->keyword, 0);
    }

    int* r = malloc(sizeof *r);
    if (r == NULL) { bail("malloc failed"); }
    *r = count;
    return r;
}

ssize_t regular_str_search(const char* text, const char* keyword, size_t offset) {
    const char* p = strstr(text + offset, keyword);
    if (p == NULL) {
        return -1;
    } else {
        return (p - text);
    }
}

ssize_t stupid_str_search(const char* text, const char* keyword, size_t offset) {
    for (size_t i = offset; text[i] != '\0'; i++) {
        size_t j = 0;
        for (; text[i + j] != '\0' && keyword[j] != '\0' && text[i + j] == keyword[j]; j++) {
        }

        if (keyword[j] == '\0') {
            return i;
        }
    }

    return -1;
}

ssize_t simd_str_search(const char* text, const char* keyword, size_t offset) {
	size_t text_len = strlen(text);
	size_t keyword_len = strlen(keyword);
	if (text_len < 32 || keyword_len > 32) {
		return -1;
	}
	unsigned char keyword_padded[32] = {0};
	unsigned char keyword_mask[32] = {0};
	const char* keyword_p = keyword;
	for (size_t i = 0; i < keyword_len; i++) {
		keyword_padded[i] = keyword_p[i];
		keyword_mask[i] = 0xFF;
	}
	__m256i keywordv = _mm256_loadu_si256((__m256i*)keyword_padded);
	__m256i mask = _mm256_loadu_si256((__m256i*)keyword_mask);
	// print_avx_vector("keywordv", keywordv);
	// print_avx_vector("mask", mask);

	const char* text_p = text;
	for (size_t i = offset; i <= text_len - 32; i++) {
		// printf("index: %lu\n", i);
		__m256i textv = _mm256_loadu_si256((__m256i*)(text_p + i));
		// print_avx_vector("textv", textv);
		__m256i r1 = _mm256_xor_si256(textv, keywordv);
		// print_avx_vector("r1", r1);
		__m256i r2 = _mm256_and_si256(r1, mask);
		// print_avx_vector("r2", r2);
		if (_mm256_testz_si256(r2, r2)) {
			return i;
		}
	}

	return -1;
}

void usage() {
    fputs("usage: ./search -keyword KEYWORD -directory DIR [-simd] [-print]\n", stderr);
    exit(EXIT_FAILURE);
}

int main(int argc, char* argv[]) {
    int use_simd = 0;
    int use_stupid = 0;
    int print_matches = 0;
    int parallel;
    char* keyword = NULL;
    char* directory = NULL;

    int i = 1;
    while (i < argc) {
        if (strcmp(argv[i], "-simd") == 0) {
            use_simd = 1;
            i++;
        } else if (strcmp(argv[i], "-print") == 0) {
            print_matches = 1;
            i++;
        } else if (strcmp(argv[i], "-parallel") == 0) {
            parallel = 1;
            i++;
        } else if (strcmp(argv[i], "-stupid") == 0) {
            use_stupid = 1;
            i++;
        } else if (strcmp(argv[i], "-keyword") == 0) {
            if (i + 1 >= argc) {
                usage();
            }
            keyword = argv[i + 1];
            i += 2;
        } else if (strcmp(argv[i], "-directory") == 0) {
            if (i + 1 >= argc) {
                usage();
            }
            directory = argv[i + 1];
            i += 2;
        } else {
            usage();
        }
    }

    if (keyword == NULL || directory == NULL) {
        usage();
    }

    struct corpus corpus = load_all_texts(directory);
    str_search_t search_f = use_stupid ? stupid_str_search : (use_simd ? simd_str_search : regular_str_search);

    clock_t start = clock();
    int count = 0;
    if (parallel) {
        pthread_t thrd1, thrd2;
        size_t midpoint = corpus.length / 2;
        struct thread_context ctx1 = { .search_f = search_f, .corpus = corpus, .start = 0, .end = midpoint, .keyword = keyword };
        struct thread_context ctx2 = { .search_f = search_f, .corpus = corpus, .start = midpoint, .end = corpus.length, .keyword = keyword };
        int r = pthread_create(&thrd1, NULL, search_thread, &ctx1);
        if (r < 0) { bail("pthread_create failed"); }
        r = pthread_create(&thrd2, NULL, search_thread, &ctx2);
        if (r < 0) { bail("pthread_create failed"); }

        void* retval;
        r = pthread_join(thrd1, &retval);
        if (r < 0) { bail("pthread_join failed"); }
        count += *(int*)retval;

        r = pthread_join(thrd2, &retval);
        if (r < 0) { bail("pthread_join failed"); }
        count += *(int*)retval;
    } else {
        for (size_t i = 0; i < corpus.length; i++) {
            count += search_one(search_f, corpus.texts[i], keyword, print_matches);
        }
    }
    clock_t end = clock();
    float duration_ms = (float)(end - start) / CLOCKS_PER_SEC * 1000;
    
    printf("%d in %.1f ms\n", count, duration_ms);
    return 0;
}
