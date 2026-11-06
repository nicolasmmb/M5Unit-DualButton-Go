# PLAN — M5Unit-DualButton-Go

Driver **100% Go** para o **M5Stack Unit Dual Button (SKU U025)**.

- Módulo: `github.com/nicolasmmb/M5Unit-DualButton-Go`
- Diretório: `/Users/nicolas.mmb/__DEV__/M5Unit-DualButton-Go`
- Padrão de referência: `m5stick-hardware-pocs/docs/LIB-CREATION-GUIDE.md`
- Status: **Fases 0–4 concluídas**; falta publicar (tag/repo) e validar em
  hardware (Fase 5)

---

## 1. Objetivo

Entregar uma lib Go/TinyGo para o Unit Dual Button com:

1. Pin injetável (`Get() bool`) — roda em TinyGo e em teste de host, sem CGo.
2. Debounce + máquina de estados de eventos por botão: press, release, click,
   double click, multi click, hold start/tick/end.
3. Zero alocação no caminho crítico (`Update`) e footprint pequeno.
4. Relógio injetável (`Clock`) para testar transições sem `sleep`.
5. Paridade funcional com `hardware.Button` da UIFlow2 (eventos) e leitura raw
   do exemplo Arduino oficial (`digitalRead`).

---

## 2. Pesquisa de hardware (concluída)

### 2.1 Identidade

| Item | Valor |
|---|---|
| Produto | Unit Dual Button |
| SKU | **U025** |
| Interface | **GPIO** (não é I2C, não é UART, não tem MCU/firmware) |
| Conector | HY2.0-4P (Grove), usa **PORT B** |
| Peso / tamanho | 8 g / 48 × 24 × 8 mm |
| Doc oficial | https://docs.m5stack.com/en/unit/dual_button |
| Schematic | `Sch_UNIT_DualButton_v1.1.pdf` |

### 2.2 Pinagem (PORT B)

| Grove | Cor | Função | M5StickC Plus2 |
|---|---|---|---|
| GND | preto | GND | — |
| 5V | vermelho | VCC | — |
| Yellow | amarelo | **Red button** | **G26** |
| White | branco | **Blue button** | **G36** |

Confirmado pela UIFlow2 (`DualButtonUnit((36, 26))` retorna `blue, red`) e pelo
exemplo Arduino oficial (`pinMode(26, INPUT); pinMode(36, INPUT);`).

### 2.3 Elétrica (schematic v1.1)

Cada botão é um tact switch para GND com **divisor no próprio unit**:

```text
5V ──[ R1 10K ]──┬── net A (Grove) ──┬── SW1 ── GND
                 │                   │
              [ R2 20K ]              └─ C1 100nF
                 │
                GND
```

- Solto: `5 V × 20K / (10K + 20K)` = **≈ 3,33 V** → seguro para GPIO ESP32.
- Pressionado: **≈ 0 V** (switch curto-circuita A para GND), `I ≈ 0,5 mA`.
- **Ativo em nível baixo**, com **pull-up/pull-down externos** já no unit.
- Consequência: **não precisa (nem deve) habilitar pull-up interno**.

### 2.4 Pinos críticos no ESP32

- **G36 é input-only** (`SENSOR_VP`) e **não tem pull-up/pull-down interno**.
  A lib **não pode exigir** pull-up interno — o divisor do unit resolve.
- G26 é GPIO normal (DAC/ADC2, irrelevante aqui).
- Nenhum dos dois aceita configuração de saída: helper de configuração deve
  usar `machine.PinInput` puro.

### 2.5 Semântica oficial (UIFlow2 / MicroPython)

`hardware.Button` (base `driver/button.py`):

| Constante | Valor | Significado |
|---|---|---|
| `NO_ACTIVE` | 0 | idle |
| `HOLD` | 1 | pressionado além de `press_ticks` |
| `PRESSED` | 2 | pressionou (após debounce) |
| `RELEASED` | 3 | soltou |
| `CLICKED` | 4 | clique simples |
| `DOUBLE_CLICKED` | 5 | clique duplo |

Timings default: `debounce = 50 ms`, `click window = 400 ms`,
`hold = 800 ms`, `max_clicks = 0` (multi-click desligado).
Callbacks: click, double-click, multi-click, long-press start/stop,
during-long-press.

`SimpleButton` / `SimpleDualButtonUnit`: leitura crua (`value()`,
`is_active()`) + bordas debounced (`was_pressed()`, `was_released()`).

### 2.6 O que **não** existe

Sem registradores, sem ID de modelo, sem init, sem endereço, sem LEDs, sem
FIFO. Não há o que "validar" no hardware além de ler o pino — logo a lib é
totalmente testável em host com um pino falso.

---

## 3. Escopo

### 3.1 Dentro

- `Device` com dois botões nomeados (`Blue`, `Red`).
- Leitura raw + debounce por botão.
- Eventos por bitmask: `Pressed`, `Released`, `Click`, `DoubleClick`,
  `MultiClick`, `HoldStart`, `HoldTick`, `HoldEnd`.
- Timings configuráveis (debounce, janela de click, hold, max clicks).
- `Clock` injetável; default `time.Now()`.
- Helper `Configure` para `machine.Pin` em `//go:build esp32`.
- Exemplo TinyGo (`examples/read`) para PORT B 26/36.
- Testes de host com pino e relógio falsos; contratos de 0 alloc e footprint.
- Benchmarks + `scripts/bench.sh` + CI + README/BENCHMARKS/PLAN.

### 3.2 Fora (anti-escopo, com motivo)

| Item | Motivo |
|---|---|
| Emular I2C/registradores | O unit não tem chip. |
| Goroutine interna / ticker | Consome RAM e rouba CPU em firmware com BLE + LCD; quem chama `Update` controla o ritmo. |
| ISR/IRQ como caminho principal | `machine.Pin` só expõe `SetInterrupt` no ESP32 e exige callback alocado; fica como fase opcional. |
| Callbacks de evento no core | API de polling evita reentrância e alocação; paridade com `setCallback` fica coberta por eventos retornados (ver §5.4). |
| Suporte a clique "infinito" quando `maxClicks=0` | Ambíguo (sempre esperaria mais cliques). Default: 1 (= só click/double). |
| Detecção de "unit presente" | Impossível sem chip: os pinos são iguais a botões soltos. |
| LED / RGB | O unit não tem. |
| Driver de outros units passivos (Button, Mini Button) | Reaproveitam o mesmo core, mas cada SKU terá seu repo se valer a pena. |

---

## 4. Arquitetura

```text
M5Unit-DualButton-Go/
├── go.mod                  # go 1.26, zero dependências
├── LICENSE                 # MIT (copiar de M5Unit-ToF90-Go)
├── README.md
├── BENCHMARKS.md
├── PLAN.md
├── Makefile
├── .gitignore
├── .github/workflows/test.yml
├── button.go               # Button (Blue/Red), String()
├── events.go               # Events bitmask, helpers Has/Any
├── device.go               # Pin, Clock, Device, New, setters, acesso
├── state.go                # FSM por botão (debounce, clicks, hold)
├── machine_esp32.go        # //go:build esp32 — Configure(machine.Pin)
├── device_test.go          # fake pin/clock, tabela de transições
├── alloc_test.go           # AllocsPerRun + unsafe.Sizeof
├── bench_test.go           # cenários de bounce/click/hold
├── scripts/bench.sh        # copiado, adaptado de ToF90
└── examples/
    ├── read/main.go        # eventos no serial
    └── ui/main.go          # opcional: barra de hold no LCD
```

Sem `internal/`: a superfície inteira é a API pública.

### 4.1 API proposta

```go
package dualbutton

// Pin é o mínimo que a lib precisa. machine.Pin satisfaz direto.
type Pin interface{ Get() bool }

// Clock é monotônico. time.Now() satisfaz via Now() time.Time no default.
type Clock interface{ Now() time.Duration }

type Button uint8

const (
    Blue Button = iota // G36 / white
    Red                // G26 / yellow
)

type Events uint8

const (
    Pressed Events = 1 << iota // borda de descida debounced
    Released                   // borda de subida debounced
    Click                      // clique simples (após janela)
    DoubleClick                // dois cliques na janela
    MultiClick                 // 3+ cliques na janela
    HoldStart                  // passou holdTime ainda pressionado
    HoldTick                   // repetição a cada holdRepeat enquanto held
    HoldEnd                    // soltou depois do hold
)

func (e Events) Has(flag Events) bool
func (e Events) String() string

type Device struct{ /* ver §4.2 */ }

func New(blue, red Pin) (*Device, error) // ErrNilPin

// Configuração (segura para uso antes do primeiro Update).
func (d *Device) SetActiveLow(bool)        // default true
func (d *Device) SetDebounce(time.Duration) // default 50ms, mín 1ms
func (d *Device) SetClickWindow(time.Duration) // default 400ms
func (d *Device) SetHoldTime(time.Duration)    // default 800ms
func (d *Device) SetHoldRepeat(time.Duration)  // 0 = sem HoldTick
func (d *Device) SetMaxClicks(uint8)           // 1..4, default 2 (=click+double)
func (d *Device) SetClock(Clock)               // default relógio de boot

// Leitura.
func (d *Device) Update() (blue, red Events) // poll dos dois pinos, 1x relógio
func (d *Device) UpdateContext(ctx context.Context) (blue, red Events, error)
func (d *Device) Read(b Button) bool         // nível cru (pós-polaridade)
func (d *Device) Down(b Button) bool         // estado debounced
func (d *Device) Held(b Button) time.Duration // tempo pressionado (0 se solto)
func (d *Device) Reset()                     // zera FSM e re-baseline
```

`Update` lê **cada pino uma vez** e **um único `Clock.Now()`**, para os dois
botões ficarem coerentes no mesmo instante (mesmo frame de UI).

### 4.2 Estado interno (alvo de footprint)

```go
type btnState struct {
    raw        bool          // leitura crua (após polaridade)
    rawSince   int64         // ns: quando raw mudou
    stable     bool          // debounced: pressionado
    downSince  int64         // ns: início do pressionado estável
    lastRelease int64        // ns: fim do último click
    clicks     uint8         // cliques na janela corrente
    holding    bool          // HoldStart já emitido
    ticked     int64         // ns do último HoldTick
}

type Device struct {
    mu     sync.Mutex
    blue   Pin
    red    Pin
    clk    Clock
    cfg    config            // 6 durações/limites
    b      [2]btnState
    started bool             // baseline já feita
}
```

Medido: `Device = 200 B`, `btnState = 40 B`, `config = 40 B`. Orçamento no
teste: `Device <= 256 B`, `btnState <= 48 B`, `config <= 64 B`.

### 4.3 FSM por botão

Estados: `idle → down → count → idle`, mais `press` (hold) e `pressEnd`.

```text
idle:
  raw ativo                  → down (downSince=now)
down:
  raw inativo  e dt<debounce → idle              (glitch: nada é emitido)
  raw inativo  e dt>=debounce→ up; Released+Pressed? não: Pressed no início
  ainda ativo  e dt>=hold    → press; HoldStart
  (Pressed é emitido na entrada de down; Pressed só após debounce)
up:
  raw ativo  e dt<debounce   → volta ao estado anterior (bounce)
  dt>=debounce               → clicks++; count
count:
  raw ativo                  → down (Pressed; downSince=now)
  dt > clickWindow:
      clicks==1 → Click
      clicks==2 → DoubleClick
      clicks>=3 → MultiClick
      → idle
press:
  raw inativo                → pressEnd; Released
  ativo                      → HoldTick a cada holdRepeat (se != 0)
pressEnd:
  raw ativo  e dt<debounce   → volta a press
  dt>=debounce               → HeldEnd; idle
```

Regras:

- **`Pressed` só é emitido depois do debounce** (o UIFlow2 emite antes, o que
  gera falso press em bounce mecânico — desvio consciente, documentado).
- `Click`/`DoubleClick`/`MultiClick` nunca coexistem no mesmo `Update`.
- `Update` é chamado com intervalo > `debounce`? Documentar requisito:
  **≤ 20 ms** para precisão de `HoldTick` e `DoubleClick`.
- Primeiro `Update` faz **baseline** (não emite `Pressed` para botão já
  pressionado no boot) — mesmo comportamento do `OCS_INIT` oficial.

### 4.4 Configuração do pino

`machine.Pin` já satisfaz `Pin` (`Get() bool`), então não há wrapper. Em
`machine_esp32.go`:

```go
//go:build esp32
package dualbutton

func Configure(p machine.Pin) error {
    return p.Configure(machine.PinConfig{Mode: machine.PinInput})
}
```

Sem pull-up interno (divisor do unit). Para botão solto (sem unit), o usuário
configura `PinInputPullup` no seu próprio pino — documentado no README.

---

## 5. Testes

### 5.1 Pino e relógio falsos

```go
type fakePin struct{ level bool; reads int }
func (p *fakePin) Get() bool { p.reads++; return p.level }

type fakeClock struct{ ns int64 }
func (c *fakeClock) Now() time.Duration { return time.Duration(c.ns) }
```

### 5.2 Contratos

- Tabela de transições: `[]struct{ ms int64; blue, red bool; want Events }`
  cobrindo glitch (5 ms), click, double, triple, hold, hold+repeat, release
  após hold, bounce na descida e na subida.
- `TestNoAllocUpdate`: `AllocsPerRun(100, ...) == 0`.
- `TestFootprint`: limites de `Device` e `btnState`.
- `TestBaseline`: botão pressionado no boot não gera `Pressed`.
- `TestReadsPerUpdate`: exatamente 2 leituras de pino e 1 leitura de relógio.
- `TestNoPin` / `TestActiveHigh`.
- `go test -race` (mutex × 2 goroutines lendo `Down` durante `Update`).
- Propriedade: sequências aleatórias de bounce (100k iterações) nunca emitem
  `Click` sem `Pressed`/`Released` precedentes nem eventos duplicados.

### 5.3 Benchmarks (`bench_test.go`)

| Benchmark | Mede |
|---|---|
| `Update/Idle` | custo base (2 reads + 1 clock) |
| `Update/Click` | transição completa click |
| `Update/HoldTick` | path de hold com repeat |
| `Update/Bounce` | 40 trocas em 40 iterações (pior caso de FSM) |
| `Update/BothPressed` | dois botões mudando no mesmo frame |

Métricas: `ns/op`, `allocs/op` (0), `pinreads/op` (=2) e `clockreads/op` (=1).
Não se aplica `tx/op` (sem barramento) — adaptação documentada do padrão da
família.

### 5.4 Paridade com a lib oficial

| UIFlow2 | Esta lib |
|---|---|
| `isPressed()` | `Update()` retornando `Pressed` / `Down(b)` |
| `isReleased()` | `Released` |
| `wasClicked()` | `Click` |
| `wasDoubleClicked()` | `DoubleClick` |
| `wasHold()` / `isHolding()` | `HoldStart` / `Down(b)` + `Held(b)` |
| multi-click callback | `MultiClick` (`SetMaxClicks`) |
| long-press start/stop | `HoldStart` / `HoldEnd` |
| during long press | `HoldTick` (`SetHoldRepeat`) |
| `SimpleButton.value()` | `Read(b)` |
| `SimpleButton.was_pressed()` | `Pressed` (sem janela de click) |
| `setCallback` | polling dos eventos (decisão §3.2) |

---

## 6. Fases

### Fase 0 — Pesquisa e escopo ✅

- [x] Página oficial, SKU, pinagem, conector.
- [x] Schematic v1.1 (divisor 10K/20K, ativo baixo, 3,33 V).
- [x] Lib UIFlow2 (`dual_button.py`, `driver/button.py`) e exemplo Arduino.
- [x] Restrições do ESP32 (G36 input-only, sem pull).
- [x] Anti-escopo e decisões de API.

### Fase 1 — Esqueleto e core

- [x] `go mod init github.com/nicolasmmb/M5Unit-DualButton-Go`, LICENSE MIT.
- [x] `button.go`, `events.go`, `device.go` (sem FSM): `New`, setters,
      `Read`, `Down`, `Held`, `Reset`.
- [x] `state.go`: debounce puro (Pressed/Released), baseline no 1º `Update`.
- [x] `machine_esp32.go` + `examples/read`.
- **DoD:** `go vet ./...` e `go test ./...` verdes; exemplo compila com
  `tinygo build -target=m5stick-c ./examples/read`.

### Fase 2 — Eventos completos

- [x] `Click` / `DoubleClick` / `MultiClick` com janela configurável.
- [x] `HoldStart` / `HoldTick` / `HoldEnd` + `SetHoldRepeat`.
- [x] `UpdateContext` (checagem de `ctx` entre os dois botões).
- [x] Tabela completa de transições + teste de propriedade (40 seeds × 3000
      passos, invariantes de exclusividade e coerência de `Down`).
- **DoD:** todos os eventos da §5.4 cobertos por teste; 0 alloc.

### Fase 3 — Contratos, benchmarks e CI

- [x] `alloc_test.go` (alloc + footprint) e `bench_test.go`.
- [x] `scripts/bench.sh` (run/save/compare/alloc/profile/escape).
- [x] `Makefile` (`test`, `check`, `bench`, `alloc`, `footprint`, `examples`).
- [x] `.github/workflows/test.yml` (test -race + vet).
- [x] `BENCHMARKS.md` com a rodada `v0.1.0` (não houve antes/depois: a FSM já
      nasceu baseada em timestamps, sem tabela de estados por fase).
- **DoD:** CI verde; `allocs/op == 0`; `pinreads/op == 2`.

### Fase 4 — Documentação e publicação

- [x] README (o que é, pinagem, elétrica, API, performance, limitações,
      roadmap, referências).
- [ ] `git init` + `gh repo create nicolasmmb/M5Unit-DualButton-Go --public
      --source=. --remote=origin --push` (aguardando confirmação).
- [ ] Tag `v0.1.0`; commits agrupados (feat/test/docs) como no guia.
- **DoD:** `go get ...@v0.1.0` funciona a partir do repo de POC.

### Fase 5 — POC e validação de hardware

- [x] Flash + boot do `examples/read` em M5StickC Plus2
      (`/dev/cu.usbserial-5A6C0570831`): boot ok, warning de unit ausente,
      `HoldStart` em 802 ms, `HoldTick` a 200 ms no serial.
- [x] Validação funcional com o unit real via `examples/validate`
      (checklist guiado): 9/9 PASS — click, double, multi, hold, hold repeat,
      os dois juntos, 10 presses rápidos; jitter de `HoldTick` 0–1 ms.
- [x] Auto-detecção de porta no firmware de validação (probe com pull-down).
- [ ] `m5stick-hardware-pocs/units/dualbutton/main.go` consumindo a tag.
- [ ] UI: dois indicadores (azul/vermelho) + contador de eventos + serial 1 Hz.
- [ ] Repetir o checklist com o cabo no **PORT B** (G26/G36) para cobrir o
      caminho do GPIO36 sem pull-up.

**Resultado (2026-11-05):** o unit estava no **PORT A** (G32/G33), não no
PORT B. O firmware detectou isso sozinho e rodou o checklist inteiro ali:
9/9 PASS. Evidência de que não era problema de leitura: apertar o vermelho
derrubou G32 para L e voltou a H ao soltar. O PORT B ficou sem validação
porque o cabo não estava nele.

**Histórico do bloqueio anterior (2026-11-05):** com a placa conectada, G26/G36 estão
**flutuando** (medição: G26 `noPull=L`, `pullup=H`, `pulldown=L`; G32/G33
variando; sem resposta em nenhum dos pinos do Grove). Conclusão: **nada plugado
em PORT B** no momento do teste, então o unit não pôde ser validado. O driver
reagiu corretamente — pinos em nível baixo viram "ambos pressionados", e o
exemplo agora avisa isso no boot.
- **DoD:** eventos coerentes em uso real, sem falso press, sem bounce visível.

---

## 7. Matriz de validação em hardware

| Alvo | Porta | Pinos | Pull interno | O que validar |
|---|---|---|---|---|
| M5StickC Plus2 | PORT B | 26 red / 36 blue | **não usar** | caso principal; G36 sem pull |
| M5StickC Plus2 | PORT B | 26/36 | não usar | hold + double-click reais |
| M5Core / Core2 | PORT B | 26 / 36 | não usar | divisor 5 V → 3,3 V |
| AtomS3 + Grove | qualquer | ex. 1/2 | opcional | mesma lib em outro target |
| Botão solto (sem unit) | — | ex. 26 | `PinInputPullup` | `SetActiveLow(true)` default |
| Botão ativo alto | — | — | `PinInputPulldown` | `SetActiveLow(false)` |

Checklist por alvo: init/sem falso press no boot · click · double · triple ·
hold (start/tick/end) · bounce visual (sacudir) · 30 min contínuos sem lixo.

---

## 8. Riscos e mitigação

| Risco | Mitigação |
|---|---|
| G36 input-only e sem pull-up | Nunca habilitar pull; helper usa `PinInput`; documentado. |
| Divisor 10K/20K → 3,33 V no pino | Fora do escopo da lib; citar no README que é seguro. |
| Bounce de tact switch barato (>20 ms) | Debounce default 50 ms e configurável até 1 s. |
| `Update` chamado devagar (ex.: 100 ms) | Documentar requisito ≤20 ms; `Held`/`Down` continuam corretos. |
| Botão preso no boot gerando evento fantasma | Baseline no 1º `Update` (`Reset` re-baseline). |
| Multi-click esperando indefinidamente | `SetMaxClicks` default 2; nunca "infinito". |
| `clock.Now()` por botão gerando incoerência | Um único `Now()` por `Update`. |
| Interface `Pin` fazendo escape no `Update` | `Pin` guardado no `Device` (sem slice/alocação); teste de alloc cobre. |
| `time.Now()` no TinyGo custa mais que contador | Gancho `SetClock` permite clock barato (ex.: `machine` ticks) no futuro. |
| Dois `Update` concorrentes | `sync.Mutex`; documentado que `Update` é serializado. |

---

## 9. Comparação com as libs oficiais

| Aspecto | Arduino (oficial) | UIFlow2 `hardware.Button` | Esta lib |
|---|---|---|---|
| Debounce | nenhum (só compara valores) | 50 ms fixo/config | 50 ms default, configurável |
| Eventos | nenhum | 6 estados + callbacks | 8 flags de evento, polling |
| Multi-click | não | sim (desligado por default) | sim, `SetMaxClicks` 1–4 |
| Hold repeat | não | "during long press" | `HoldTick` com `SetHoldRepeat` |
| Testável em host | não | não | sim (pino + relógio falsos) |
| Alocação | n/a | n/a | 0 no `Update` |
| Multi-target | só Arduino | só MicroPython | qualquer target TinyGo |

---

## 10. Decisões (aceitas em 2026-11-05, "vai com as propostas")

1. **Nome do módulo**: `M5Unit-DualButton-Go` — segue a família. ✅
2. **`New` retorna `(*Device, error)`** com `ErrNilPin`; desvia de
   `New(bus) *Device`, mas evita pânico. ✅
3. **`HoldTick` desligado por default** (`SetHoldRepeat(0)`). ✅
4. **Sem callbacks no core**; `SetHandler` fica para uma v0.2.0 se houver
   demanda (não quebra a API). ✅
5. **`UpdateContext` mantido** por simetria; checa `ctx` entre os dois botões
   (as leituras são non-blocking, então o ganho real é saída antecipada). ✅

Desvios do plano durante a implementação, já refletidos no código:

- `ConfigureInput` não retorna `error`: `machine.Pin.Configure` do TinyGo 0.44
  não retorna erro (`p.Configure(machine.PinConfig{Mode: machine.PinInput})`).
- Baseline: um botão pressionado no boot **não** gera `Pressed`, mas o release
  conta como clique (paridade com `OCS_INIT`/`OCS_COUNT` da UIFlow2).
- Janela de clique conta a partir da **borda física de release**
  (`rawSince`), não do `Released` pós-debounce — `Click` chega ~`clickWindow`
  após o dedo sair.

---

## 11. Referências

- Doc oficial: https://docs.m5stack.com/en/unit/dual_button
- Schematic: `Sch_UNIT_DualButton_v1.1.pdf` (m5stack-doc OSS)
- UIFlow2 Dual Button: https://uiflow-micropython.readthedocs.io/en/latest/unit/dualbutton.html
- `uiflow-micropython/m5stack/libs/unit/dual_button.py`
- `uiflow-micropython/m5stack/libs/driver/button.py`
- Arduino oficial: `M5-ProductExampleCodes/Unit/DUAL_BUTTON/Arduino/dual_button/dual_button.ino`
- ESP32 G36 input-only: datasheet ESP32 (SENSOR_VP, sem pull interno)
