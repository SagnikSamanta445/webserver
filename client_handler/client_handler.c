#include "client_handler.h"
#include "cache.h"
#include "http.h"
#include "proxy_parse.h"

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

    /* -------- CACHE CHECK -------- */

    char* tempReq = strdup(buffer);
    if (!tempReq) {
        free(buffer);
        return;
    }

    char* cached_data = NULL;
    int cached_size = 0;

    if (cache_get(tempReq, &cached_data, &cached_size))
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

        free(cached_data);
    }
    else
    {
        struct ParsedRequest* request =
            ParsedRequest_create();

        if (ParsedRequest_parse(request,
                                buffer,
                                total_bytes) == 0)
        {
            if (!strcmp(request->method, "CONNECT"))
            {
                handle_connect(socket, request);
            }
            else if (!strcmp(request->method, "GET") &&
                request->host &&
                request->path &&
                checkHTTPversion(request->version) == 1)
            {
                if (handle_request(socket,
                           request,
                           tempReq) == -1)
                {
                    sendErrorMessage(socket, 500);
                }
            }
            else
            {
                sendErrorMessage(socket, 400);
            }
        }
        else
        {
            sendErrorMessage(socket, 400);
        }

        ParsedRequest_destroy(request);
    }

    

    free(tempReq);
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

    tunnel_data(clientSocket, remoteSocket);

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

        if (select(maxfd,
                   &readfds,
                   NULL,
                   NULL,
                   NULL) <= 0)
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