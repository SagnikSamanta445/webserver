#ifndef CACHE_H
#define CACHE_H

#include <stddef.h>

#define MAX_CACHE_SIZE (200*(1 << 20))
#define HASH_SIZE 1024

typedef struct cache_node {
        char* url;                 // URL string
        char* data;                // Cached data
        int size;                  // Size of cached data

        struct cache_node* prev;   // Previous node in LRU list
        struct cache_node* next;   // Next node in LRU list

        struct cache_node* hash_next;  // Next node in hash bucket to handle collisions

}cache_node;

void cache_init();                                                    // Initialize the cache system
int cache_get(const char* url, char** data_out, int* size_out );      // Retrieve cached data for a given URL, returns 1 if found, 0 otherwise
void cache_put(const char* url, const char* data, int size);          // Add data to the cache for a given URL, evicting least recently used items if necessary

double cache_hit_ratio();
void cache_print_stats();

#endif