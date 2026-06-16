#include "cache.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>

static cache_node* hash_table[HASH_SIZE];

static cache_node* head = NULL;
static cache_node* tail = NULL;

static size_t current_cache_size = 0;
static size_t entry_count = 0;
static size_t eviction_count = 0;
static size_t total_lookups = 0;
static size_t total_hits = 0;
static pthread_mutex_t cache_lock;


/*-------------------HASH FUNCTION -------------------*/
static unsigned int hash_func(const char* str)
{
    unsigned int hash = 5381;

    while(*str)
        hash = ((hash << 5) + hash) + *str++;
    
    return hash % HASH_SIZE;
}

/*-------------------DLL helpers----------------------*/
static void remove_from_list(cache_node* node)
{
    if(!node) return;

    if(node->prev) node->prev->next = node->next;

    if(node->next) node->next->prev = node->prev;

    if(node == head) head = node->next;

    if(node == tail) tail = node->prev;

    node->prev = NULL;
    node->next = NULL;
}

static void insert_at_head(cache_node* node)
{
    node->prev = NULL;
    node->next = head;

    if (head)
        head->prev = node;

    head = node;

    if (!tail)
        tail = node;
}


static void move_to_front(cache_node* node)
{
    if (node == head)
        return;

    remove_from_list(node);
    insert_at_head(node);
}

// Evict least recently used item from cache.
// Logic to remove from hash table and DLL, and free memory.
static void evict_lru()
{
    if (!tail)
        return;

    cache_node* node = tail;

    /* Remove from hash table */
    unsigned int index = hash_func(node->url);
    printf("EVICTING: %s (%d bytes)\n",node->url,node->size);
    
    cache_node** curr = &hash_table[index];
    while (*curr && *curr != node)
        curr = &(*curr)->hash_next;

    if (*curr)
        *curr = node->hash_next;

    /* Remove from DLL */
    remove_from_list(node);
    //update counts
    eviction_count++;
    entry_count--;

    current_cache_size -= node->size;

    free(node->url);
    free(node->data);
    free(node);
}

void cache_init()
{
    memset(hash_table, 0, sizeof(hash_table));
    head = NULL;
    tail = NULL;
    current_cache_size = 0;
    pthread_mutex_init(&cache_lock, NULL);
}

int cache_get(const char* url, char** data_out, int* size_out)
{
    pthread_mutex_lock(&cache_lock);

    total_lookups++;

    unsigned int index = hash_func(url);

    cache_node* node = hash_table[index];

    while (node)
    {
        if (strcmp(node->url, url) == 0)
        {
            total_hits++;

            move_to_front(node);

            *size_out = node->size;

            *data_out = malloc(node->size);
            if (!*data_out)
            {
                pthread_mutex_unlock(&cache_lock);
                return 0;
            }

            memcpy(*data_out, node->data, node->size);

            pthread_mutex_unlock(&cache_lock);
            return 1;
        }

        node = node->hash_next;
    }

    pthread_mutex_unlock(&cache_lock);
    return 0;
}


void cache_put(const char* url, const char* data, int size)
{
    if (size <= 0 || size > MAX_CACHE_SIZE)
        return;

    pthread_mutex_lock(&cache_lock);

    unsigned int index = hash_func(url);
    cache_node* node = hash_table[index];

    /* -------- 1. CHECK IF ALREADY EXISTS -------- */

    while (node)
    {
        if (strcmp(node->url, url) == 0)
        {
            current_cache_size -= node->size;
            current_cache_size += size;

            free(node->data);
            node->data = malloc(size);
            if (!node->data) {
                pthread_mutex_unlock(&cache_lock);
                return;
            }

            memcpy(node->data, data, size);
            node->size = size;

            move_to_front(node);

            while (current_cache_size  > MAX_CACHE_SIZE)
                evict_lru();

            pthread_mutex_unlock(&cache_lock);
            return;
        }

        node = node->hash_next;
    }

    /* -------- 2. INSERT NEW NODE -------- */

    while (current_cache_size + size > MAX_CACHE_SIZE)
        evict_lru();

    cache_node* new_node = malloc(sizeof(cache_node));
    if (!new_node) {
        pthread_mutex_unlock(&cache_lock);
        return;
    }

    new_node->url = strdup(url);
    new_node->data = malloc(size);

    if (!new_node->url || !new_node->data)
    {
        free(new_node->url);
        free(new_node->data);
        free(new_node);
        pthread_mutex_unlock(&cache_lock);
        return;
    }

    memcpy(new_node->data, data, size);
    new_node->size = size;

    insert_at_head(new_node);

    new_node->hash_next = hash_table[index];
    hash_table[index] = new_node;

    entry_count++;
    current_cache_size += size;

    pthread_mutex_unlock(&cache_lock);
}


double cache_hit_ratio() {
    pthread_mutex_lock(&cache_lock);
    size_t lk = total_lookups;
    size_t ht = total_hits;
    pthread_mutex_unlock(&cache_lock);
    if (lk == 0) return 0.0;
    return (double)ht / lk;
}


void cache_print_stats() {
    pthread_mutex_lock(&cache_lock);
    size_t lk = total_lookups;
    size_t ht = total_hits;
    pthread_mutex_unlock(&cache_lock);
    printf("Cache lookups: %zu\n", lk);
    printf("Cache hits: %zu\n", ht);
    printf("Hit ratio: %.2f%%\n", (lk ? 100.0 * ht / lk : 0.0));
}

size_t cache_entry_count() {
    pthread_mutex_lock(&cache_lock);
    size_t n = entry_count;
    pthread_mutex_unlock(&cache_lock);
    return n;
}

size_t cache_current_size() {
    pthread_mutex_lock(&cache_lock);
    size_t s = current_cache_size;
    pthread_mutex_unlock(&cache_lock);
    return s;
}

size_t cache_eviction_count() {
    pthread_mutex_lock(&cache_lock);
    size_t e = eviction_count;
    pthread_mutex_unlock(&cache_lock);
    return e;
}