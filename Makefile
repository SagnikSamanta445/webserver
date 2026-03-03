# Compiler
CC = clang

# Base compiler flags
BASE_FLAGS = -pthread -g -Wall -Wextra

# Sanitizer options: none, address, thread for memory and thread error detection
SAN ?= none

ifeq ($(SAN), address)
    CFLAGS = $(BASE_FLAGS) -fsanitize=address
else ifeq ($(SAN), thread)
    CFLAGS = $(BASE_FLAGS) -fsanitize=thread
else
    CFLAGS = $(BASE_FLAGS)
endif

# Include directories
INCLUDES = -Icache \
           -Iclient_handler \
           -Ihttp \
           -Iproxy_parse \
           -Iserver \
					 


# Source files
SRC = main.c \
      server/server.c \
      cache/cache.c \
      client_handler/client_handler.c \
      http/http.c \
      proxy_parse/proxy_parse.c \
			server/thread_pool.c

# Object files
OBJ = $(SRC:.c=.o)

# Output binary
TARGET = proxy

# Build target
$(TARGET): $(OBJ)
	$(CC) $(CFLAGS) $(INCLUDES) -o $(TARGET) $(OBJ)

# Compile rule
%.o: %.c
	$(CC) $(CFLAGS) $(INCLUDES) -c $< -o $@

# Clean rule
clean:
	rm -f */*.o $(TARGET)

# Rebuild shortcut
rebuild: clean $(TARGET)