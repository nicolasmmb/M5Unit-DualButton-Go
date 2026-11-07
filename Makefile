TARGET ?= m5stick-c
TINYGO ?= tinygo
COUNT ?= 8
BENCH ?= .
BENCHTIME ?= 1s

.PHONY: test vet fmt check bench bench-save bench-compare alloc footprint escape profile examples validate agnostic

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

check: vet test alloc examples

# Run every benchmark with alloc stats.
bench:
	COUNT=$(COUNT) BENCH=$(BENCH) BENCHTIME=$(BENCHTIME) ./scripts/bench.sh run

# make bench-save NAME=before
bench-save:
	COUNT=$(COUNT) BENCH=$(BENCH) BENCHTIME=$(BENCHTIME) ./scripts/bench.sh save $(NAME)

# make bench-compare FROM=before TO=after
bench-compare:
	./scripts/bench.sh compare $(FROM) $(TO)

# Allocation contract + RAM footprint tests.
alloc:
	./scripts/bench.sh alloc

footprint:
	go test -run TestFootprint -v

escape:
	./scripts/bench.sh escape

profile:
	./scripts/bench.sh profile

examples:
	$(TINYGO) build -target=$(TARGET) -size short -o /tmp/m5unit-dualbutton-read.bin ./examples/read
	$(TINYGO) build -target=$(TARGET) -size short -o /tmp/m5unit-dualbutton-validate.bin ./examples/validate

# Guided hardware checklist: detects the Grove port, then asks for one gesture
# at a time (backlight: 1 blink = new step, 2 = PASS, 5 = FAIL).
validate:
	$(TINYGO) flash -target=$(TARGET) -port=$(PORT) -size short -monitor ./examples/validate

# Board agnosticism + chip tag coverage, no hardware required.
agnostic:
	@rm -rf /tmp/db-agn && mkdir -p /tmp/db-agn && cd /tmp/db-agn && \
	  MOD=$(shell awk '/^module /{print $$2}' go.mod) && \
	  printf 'module agn\n\ngo 1.26\n\nrequire %s v0.0.0\nreplace %s => %s\n' "$$MOD" "$$MOD" "$(CURDIR)" > go.mod && \
	  printf 'package main\n\nimport lib "%s"\n\nvar _ = lib.New\nvar _ = lib.NewLayout\nvar _ = lib.Choose\n\nfunc main() {}\n' "$$MOD" > main.go && \
	  go mod tidy >/dev/null && \
	  $(TINYGO) build -target=pico -o /tmp/db-agn/pico.bin . && \
	  $(TINYGO) build -target=arduino-nano33 -o /tmp/db-agn/nrf.bin . && \
	  printf 'package main\n\nimport lib "%s"\n\nvar _ = lib.ConfigureInput\nvar _ = lib.Detect\n\nfunc main() {}\n' "$$MOD" > main.go && \
	  go mod tidy >/dev/null && \
	  for t in m5stick-c esp32-c3-devkit-rust-1 m5stamp-s3a pico; do \
	    $(TINYGO) build -target=$$t -o /tmp/db-agn/$$t.bin . || exit 1; \
	  done && \
	  echo "agnostic: core pico+nrf OK, ConfigureInput/Detect esp32/c3/s3/rp2040 OK"
