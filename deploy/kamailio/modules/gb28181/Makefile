include ../../Makefile.defs

auto_gen=
NAME=gb28181.so
DEFS += $(shell pkg-config --cflags libxml-2.0)
LIBS=-lhiredis -ljansson $(shell pkg-config --libs libxml-2.0)

include ../../Makefile.modules
