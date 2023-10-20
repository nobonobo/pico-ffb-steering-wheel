TARGET=pico
TINYGO=tinygo
NAME=diy-ffb-wheel
TAGS=-tags dummy -tags debug
OPTIONS=-target $(TARGET)
ifneq (TAGS, "")
OPTIONS+=$(TAGS)
endif
export OPTIONS
-include .env

.PHONY: build all flash wait mon

build:
	mkdir -p build
	$(TINYGO) build $(OPTIONS) -o build/$(NAME).elf .

all: flash wait monitor

flash:
	$(TINYGO) flash $(OPTIONS) .

wait:
	sleep 2

mon:
	$(TINYGO) monitor $(OPTIONS)

gdb:
	$(TINYGO) gdb -x $(OPTIONS) -programmer=jlink 

server:
	"C:\Program Files\SEGGER\JLink\JLinkGDBServer.exe" -if swd -port 3333 -speed 4000 -device rp2040_m0_0 &

docker:
	docker compose up build 