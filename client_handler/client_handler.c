#include "client_handler.h"
#include "cache.h"
#include "http.h"
#include "proxy_parse.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>

#define MAX_BYTES 4096

void handle_client(int socket)
{
    char* buffer = calloc(MAX_BYTES, 1);
    if (!buffer) {
        return;
    }

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

    if (find(tempReq, &cached_data, &cached_size))
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
        struct ParsedRequest* request = ParsedRequest_create();

        if (ParsedRequest_parse(request,
                                buffer,
                                total_bytes) == 0)
        {
            if (!strcmp(request->method, "GET") &&
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