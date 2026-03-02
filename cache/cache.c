#include "cache.h"
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <thread_pool.h>

/* ---------------- PRIVATE GLOBALS ---------------- */

static cache_element* head = NULL;
static int cache_size = 0;
static pthread_mutex_t lock;

/* ---------------- INIT ---------------- */

void cache_init() {
    pthread_mutex_init(&lock, NULL);
}

/* ---------------- INTERNAL LRU REMOVE ---------------- */

static void remove_cache_element() {
    if (head == NULL)
        return;

    cache_element* prev = NULL;
    cache_element* curr = head;

    cache_element* lru = head;
    cache_element* lru_prev = NULL;

    time_t min_time = head->lru_time_track;

    while (curr != NULL) {
        if (curr->lru_time_track < min_time) {
            min_time = curr->lru_time_track;
            lru = curr;
            lru_prev = prev;
        }

        prev = curr;
        curr = curr->next;
    }

    if (lru_prev == NULL)
        head = lru->next;
    else
        lru_prev->next = lru->next;

    cache_size -= (lru->len +
                   sizeof(cache_element) +
                   strlen(lru->url) + 1);

    free(lru->data);
    free(lru->url);
    free(lru);
}

/* ---------------- FIND ---------------- */

int find(char* url, char** data_copy, int* size_copy) {

    pthread_mutex_lock(&lock);

    cache_element* site = head;

    while (site != NULL) {
        if (strcmp(site->url, url) == 0) {

            /* Update LRU timestamp */
            site->lru_time_track = time(NULL);

            *size_copy = site->len;

            *data_copy = malloc(site->len);
            if (!(*data_copy)) {
                pthread_mutex_unlock(&lock);
                return 0;
            }

            memcpy(*data_copy, site->data, site->len);

            pthread_mutex_unlock(&lock);
            return 1;
        }

        site = site->next;
    }

    pthread_mutex_unlock(&lock);
    return 0;
}

/* ---------------- ADD ---------------- */

int add_cache_element(char* data, int size, char* url) {

    int element_size =
        size + strlen(url) + sizeof(cache_element) + 1;

    if (element_size > MAX_ELEMENT_SIZE)
        return 0;

    pthread_mutex_lock(&lock);

    /* Evict until space available */
    while (cache_size + element_size > MAX_SIZE)
        remove_cache_element();

    cache_element* new_node =
        malloc(sizeof(cache_element));

    if (!new_node) {
        pthread_mutex_unlock(&lock);
        return 0;
    }

    new_node->data = malloc(size);
    new_node->url = strdup(url);

    if (!new_node->data || !new_node->url) {
        free(new_node->data);
        free(new_node->url);
        free(new_node);
        pthread_mutex_unlock(&lock);
        return 0;
    }

    memcpy(new_node->data, data, size);

    new_node->len = size;
    new_node->lru_time_track = time(NULL);

    /* Insert at head */
    new_node->next = head;
    head = new_node;

    cache_size += element_size;

    pthread_mutex_unlock(&lock);

    return 1;
}