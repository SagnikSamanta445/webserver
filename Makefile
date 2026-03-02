# Compiler
CC = clang

# Flags
CFLAGS = -pthread -g -Wall -Wextra

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