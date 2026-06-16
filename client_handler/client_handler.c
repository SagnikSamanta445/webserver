#include "client_handler.h"
#include "cache.h"
#include "http.h"
#include "proxy_parse.h"
#include "stats.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/select.h>
#include <arpa/inet.h>

#define MAX_BYTES 4096

static int handle_connect(int clientSocekt, 
                          struct ParsedRequest* request);
static void tunnel_data(int clientSocket, int remoteSocket);

void handle_client(int socket)
{
    char* buffer = calloc(MAX_BYTES, 1);
    if (!buffer)
        return;

    int total_bytes = 0;
    int bytes_recv;

    /* -------- RECEIVE REQUEST -------- */

    while ((bytes_recv =
            recv(socket,
                 buffer + total_bytes,
                 MAX_BYTES - total_bytes,
                 0)) > 0)
    {
        total_bytes += bytes_recv;

        if (strstr(buffer, "\r\n\r\n"))
            break;

        if (total_bytes >= MAX_BYTES)
            break;
    }

    if (total_bytes < MAX_BYTES)
        buffer[total_bytes] = '\0';
    else
        buffer[MAX_BYTES - 1] = '\0';

    if (bytes_recv <= 0)
    {
        free(buffer);
        return;
    }

    /* -------- PARSE REQUEST ------ */
    struct ParsedRequest* request = ParsedRequest_create();
    if(!request)
    {
        free(buffer);
        return;
    }

    if(ParsedRequest_parse(request, buffer, total_bytes) != 0){
        stats_record_error();
        sendErrorMessage(socket, 400);
        ParsedRequest_destroy(request);
        free(buffer);
        return;
    }

    if(!strcmp(request->method, "CONNECT"))
    {
        stats_record_connect();
        handle_connect(socket, request);
        ParsedRequest_destroy(request);
        free(buffer);
        return;
    }

    if(strcmp(request->method, "GET") != 0 || !request->host || !request->path ||
              checkHTTPversion(request->version) != 1)
    {
        stats_record_error();
        sendErrorMessage(socket, 400);
        ParsedRequest_destroy(request);
        free(buffer);
        return;
    }

    stats_record_get();

    char cache_key[2048];
    snprintf(cache_key, sizeof(cache_key), "%s%s", request->host, request->path);

    /* -------- CACHE CHECK -------- */

    char* cached_data = NULL;
    int cached_size = 0;

    if (cache_get(cache_key, &cached_data, &cached_size))
    {
        int sent = 0;

        while (sent < cached_size)
        {
            int chunk =
                (cached_size - sent > MAX_BYTES)
                ? MAX_BYTES
                : cached_size - sent;

            send(socket, cached_data + sent, chunk, 0);
            sent += chunk;
        }

        stats_record_bytes_cache(cached_size);

        free(cached_data);
    }
    else 
    {
        if(handle_request(socket, request, cache_key) == -1)
        {
            stats_record_error();
            sendErrorMessage(socket, 500);
        }
    }
    
    ParsedRequest_destroy(request);
    free(buffer);
}

static int handle_connect(int clientSocket,
                          struct ParsedRequest* request)
{
    int server_port =
        request->port ? atoi(request->port) : 443;

    int remoteSocket =
        connectRemoteServer(request->host,
                            server_port);

    if (remoteSocket < 0)
    {
        sendErrorMessage(clientSocket, 500);
        return -1;
    }

    const char* response =
        "HTTP/1.1 200 Connection Established\r\n\r\n";

    send(clientSocket,
         response,
         strlen(response),
         0);

    //printf("CONNECT START %s:%d\n",request->host,server_port);

    tunnel_data(clientSocket, remoteSocket);

    //printf("CONNECT END %s:%d\n",request->host,server_port);

    close(remoteSocket);
    return 0;
}

static void tunnel_data(int clientSocket,
                        int remoteSocket)
{
    fd_set readfds;
    char buffer[8192];

    while (1)
    {
        FD_ZERO(&readfds);
        FD_SET(clientSocket, &readfds);
        FD_SET(remoteSocket, &readfds);

        int maxfd =
            (clientSocket > remoteSocket ?
             clientSocket : remoteSocket) + 1;

        struct timeval tv = { .tv_sec = 2, .tv_usec = 0 };
        if (select(maxfd,
                   &readfds,
                   NULL,
                   NULL,
                   &tv) <= 0)
        {
            break;
        }

        /* Client → Server */
        if (FD_ISSET(clientSocket, &readfds))
        {
            int bytes =
                recv(clientSocket,
                     buffer,
                     sizeof(buffer),
                     0);

            if (bytes <= 0)
                break;

            send(remoteSocket,
                 buffer,
                 bytes,
                 0);
        }

        /* Server → Client */
        if (FD_ISSET(remoteSocket, &readfds))
        {
            int bytes =
                recv(remoteSocket,
                     buffer,
                     sizeof(buffer),
                     0);

            if (bytes <= 0)
                break;

            send(clientSocket,
                 buffer,
                 bytes,
                 0);
        }
    }
}