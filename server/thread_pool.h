#ifndef THREAD_POOL_H
#define THREAD_POOL_H

void thread_pool_init(int num_threads);
void thread_pool_add_task(int client_socket);

#endif