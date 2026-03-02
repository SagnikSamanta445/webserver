#ifndef CACHE_H
#define CACHE_H

#include <time.h>

#define MAX_SIZE 200*(1<<20)
#define MAX_ELEMENT_SIZE 10*(1<<20)

typedef struct cache_element {
    char* data;
    int len;
    char* url;
    time_t lru_time_track;
    struct cache_element* next;
} cache_element;

void cache_init();
int find(char* url, char** data_copy, int* size_copy);
int add_cache_element(char* data, int size, char* url);

#endif