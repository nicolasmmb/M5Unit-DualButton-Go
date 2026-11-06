# Benchmarks — M5Unit-DualButton-Go

## Método

- Host: Apple Silicon (darwin/arm64), Go 1.26.3, `-count=6 -benchtime=500ms`.
- Pinos e relógio **falsos** (`fakePin`, `fakeClock`): o custo medido é o da
  máquina de estados, não o da GPIO real. Isso é intencional — o `Update` não
  tem I/O bloqueante, e o `Clock` é injetável justamente para isolar o custo
  do `time.Now` (medido à parte em `BenchmarkUpdateRealClock`).
- Não existe `tx/op` (não há barramento). No lugar, cada benchmark reporta
  **`pinblue/op`**, **`pinred/op`** e **`clock/op`** — o “contrato de I/O” da
  lib deve ser sempre 1/1/1 por `Update`.
- Regenerar: `make bench` (`COUNT`, `BENCH`, `BENCHTIME` configuráveis).
  Comparar versões: `make bench-save NAME=before` → editar → `make bench-save
  NAME=after` → `make bench-compare FROM=before TO=after`.

## Resultados (`v0.1.0`)

| Benchmark | Cenário | ns/op | pinblue/op | pinred/op | clock/op | B/op | allocs/op |
|---|---|---:|---:|---:|---:|---:|---:|
| `UpdateIdle` | nada acontecendo | ~15 | 1 | 1 | 1 | 0 | 0 |
| `UpdatePoll` | loop de 100 Hz (10 ms) | ~15,4 | 1 | 1 | 1 | 0 | 0 |
| `UpdateClick` | ciclos press/release/janela | ~15,5 | 1 | 1 | 1 | 0 | 0 |
| `UpdateHoldTick` | hold com repetição de 10 ms | ~15,4–18,5 | 1 | 1 | 1 | 0 | 0 |
| `UpdateBoth` | os dois botões mudando | ~16,6 | 1 | 1 | 1 | 0 | 0 |
| `UpdateBounce` | nível trocando a cada poll | ~17,1 | 1 | 1 | 1 | 0 | 0 |
| `UpdateRealClock` | relógio default (`time.Since`) | ~50–60 | 1 | 1 | 1 | 0 | 0 |

Leitura:

- O custo é dominado pelo número de leituras, não pelo estado: “bounce” (pior
  caso lógico) custa ~2 ns a mais que o caso idle. Não há caminho com
  alocação nem com mais de uma leitura de relógio.
- `realClock` mostra que `time.Now`/`time.Since` no host custa ~35 ns — ou
  seja, **~70% do tempo do `Update` default**. Em TinyGo o custo é menor
  (contador de ticks), mas se o firmware já tem um tick barato, `SetClock`
  elimina até isso.
- A 100 Hz, o custo por segundo é ~1,5 µs de CPU (1,5 ppm do core).

## Footprint (RAM)

| Tipo | Tamanho | Orçamento no teste |
|---|---:|---:|
| `Device` | 200 B | ≤ 256 B |
| `btnState` (×2) | 40 B | ≤ 48 B |
| `config` | 40 B | ≤ 64 B |

`Device` = `sync.Mutex` (8) + 2 × `Pin` (32) + `Clock` (16) + `config` (40) +
2 × `btnState` (80) + `time.Time` do boot (24).

## Flash (TinyGo, `m5stick-c`)

| Alvo | code | data | bss | flash | ram |
|---|---:|---:|---:|---:|---:|
| `examples/read` | 39 348 | 204 | 4 708 | 39 552 B | 4 912 B |

Inclui a runtime TinyGo e o print serial; a lib em si é ~2 KB de código
(sem tabelas, sem `fmt` no caminho crítico).

## Contratos verificados

```bash
make alloc      # TestNoAllocUpdate / TestNoAllocUpdateContext / TestNoAllocAccessors / TestFootprint
make escape     # go build -gcflags=-m | grep escapes
```

- `Update`, `UpdateContext`, `Read`, `Down`, `Held`: **0 allocs/op**.
- `New` faz uma alocação (o `bootClock`); configurar `SetClock` depois não
  aloca.
- Nenhum escape de heap no caminho de polling.

## Notas de medição

- `UpdateHoldTick` teve variância maior entre rodadas (15,2–18,5 ns) por causa
  de branch prediction no caminho de `HoldTick`; a mediana fica em ~15,5 ns.
- `UpdateRealClock` usa `time.Since(time.Unix(0,0))`, que é o mesmo caminho do
  `bootClock` default.
- Emulador de pino devolve constante, então o custo real da GPIO no ESP32
  (uma escrita/leitura de registrador) é maior: some ~dezenas de ns por pin
  read. Mesmo assim, a 100 Hz isso continua irrelevante frente a BLE/LCD.

## Medições no alvo (ESP32, M5StickC Plus2)

`testing.B` é do runtime Go do host, então o que se mede na placa é **tempo de
reação e jitter**, não `ns/op`. Números do checklist em `examples/validate`,
com o unit real:

| Medida | Resultado |
|---|---|
| `HoldStart` após press (impresso 50 ms depois da borda física) | 751–753 ms → **~800 ms reais** |
| `HoldTick` com `SetHoldRepeat(200ms)` | 8 ticks, **jitter 0–1 ms** |
| `DoubleClick` (press-to-press 70–131 ms) | dispara no release, sem `Click` extra |
| `MultiClick` de 3 clicks | exatamente `janela (400 ms)` após a borda de release |
| 10 presses rápidos em 3,5 s | 5 × `DoubleClick`, **0 `Click` espúrio**, 0 evento perdido |
| Dois botões no mesmo `Update` | skew de 10 ms entre eles |
| Debounce de 50 ms em todos os gestos | nenhum falso `Pressed`/`Released` |

O relevante: a FSM no alvo entrega timing de 1 ms de jitter, e não houve um
único evento perdido/duplicado em 9 gestos seguidos.

