#include <pthread.h>
#include <stdlib.h>
#include <unistd.h>
#include "thread_pool.h"
#include "../client_handler/client_handler.h"
#include <stdio.h>

#define QUEUE_SIZE 1024

static int queue[QUEUE_SIZE];
static int front = 0;
static int rear = 0;

static pthread_mutex_t queue_lock;
static pthread_cond_t queue_not_empty;

static pthread_t *workers;
static int thread_count;

static void* worker_loop(void* arg)
{
    // Worker thread loop: continuously fetch tasks from the queue and handle them
    while (1)
    {
        pthread_mutex_lock(&queue_lock);                          // Lock the queue mutex

        while (front == rear)
            pthread_cond_wait(&queue_not_empty, &queue_lock);    // Wait until there is a task in the queue

        int client_socket = queue[front];
        front = (front + 1) % QUEUE_SIZE;                        // Move the front pointer to the next task          

        pthread_mutex_unlock(&queue_lock);                       // Unlock the queue mutex

        printf("Worker %lu handling client\n", pthread_self());
        handle_client(client_socket);                            // Handle the client request 
        close(client_socket);                                    // Close the client socket after handling
    }

    return NULL;
}

void thread_pool_init(int num_threads)
{
    // Initialize the thread pool with the specified number of worker threads
    thread_count = num_threads;

    // Initialize the queue and synchronization primitives
    pthread_mutex_init(&queue_lock, NULL);
    pthread_cond_init(&queue_not_empty, NULL);

    workers = malloc(sizeof(pthread_t) * num_threads);

    for (int i = 0; i < num_threads; i++)
        pthread_create(&workers[i], NULL, worker_loop, NULL);
}

void thread_pool_add_task(int client_socket)
{

    // Add a new client socket task to the queue for worker threads to process
    pthread_mutex_lock(&queue_lock);

    queue[rear] = client_socket;
    rear = (rear + 1) % QUEUE_SIZE;

    pthread_cond_signal(&queue_not_empty);
    pthread_mutex_unlock(&queue_lock);
}