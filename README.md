# Tarea 3 — ¿Y El Pensador? · INF-343 Sistemas Distribuidos

## Integrantes

| Nombre | Apellido | Rol |
|--------|----------|-----|
| _(Completar)_ | _(Completar)_ | _(Completar)_ |
| _(Completar)_ | _(Completar)_ | _(Completar)_ |
| _(Completar)_ | _(Completar)_ | _(Completar)_ |

---

## Índice

1. [Descripción general](#descripción-general)
2. [Arquitectura y decisiones de diseño](#arquitectura-y-decisiones-de-diseño)
3. [Estructura del repositorio](#estructura-del-repositorio)
4. [Instalación y compilación](#instalación-y-compilación)
5. [Instrucciones de uso (script.sh)](#instrucciones-de-uso-scriptsh)
6. [Formatos de archivos](#formatos-de-archivos)
7. [Consideraciones especiales](#consideraciones-especiales)
8. [Pruebas y ejemplos de salida](#pruebas-y-ejemplos-de-salida)

---

## Descripción general

El sistema implementa una red de **expendedoras distribuidas** ubicadas en Rapture. Cada expendedora es un proceso Go que corre en una de las tres máquinas virtuales asignadas al grupo (IPs: `10.10.28.35`, `10.10.28.36`, `10.10.28.37`). Los procesos se comunican directamente entre sí vía **gRPC** sin coordinador central, mantienen réplicas del inventario y la lista de vetos de todos los demás procesos, y son capaces de recuperarse de fallas aplicando un protocolo de **quorum 2/3**.

---

## Arquitectura y decisiones de diseño

### 1. Protocolo de comunicación: gRPC

Se eligió **gRPC** sobre REST o mensajería por las siguientes razones:

- **Serialización eficiente**: Protocol Buffers serializa los mensajes de inventario (listas de items con nombre y cantidad) en binario compacto, lo que es relevante al replicar estados completos entre procesos.
- **Contratos estrictos**: El archivo `.proto` define explícitamente cada mensaje y servicio, reduciendo errores de integración entre los tres nodos.
- **Comunicación directa peer-to-peer**: gRPC permite conexiones directas entre cualquier par de procesos sin pasar por un broker, lo que es consistente con el requisito de no tener coordinador central.
- **Soporte concurrente nativo**: El servidor gRPC de Go maneja múltiples conexiones entrantes en goroutines separadas, cumpliendo el requisito de escuchar instrucciones concurrentemente.

Se descartó RabbitMQ porque introduce un broker central (punto único de falla y coordinador implícito), y REST porque el overhead de HTTP/1.1 y JSON es innecesario para comunicación intra-sistema.

### 2. Descubrimiento de pares: topología estática con archivo JSON

Dado que las IPs de las máquinas virtuales son fijas y conocidas de antemano, se optó por un **archivo de topología** (`topologia.json`) generado por el script bash antes de iniciar los procesos. Este archivo lista todos los procesos del sistema con su máquina, ID y dirección `IP:puerto`.

Cada proceso:
1. Lee `topologia.json` al iniciar.
2. Intenta conectarse a todos los demás procesos listados.
3. Anuncia su propia existencia enviando `Registrar` a sus pares.
4. Espera **2 segundos** para recibir anuncios de procesos que aún no estaban listos.

Este enfoque es simple y correcto para un sistema con topología fija y conocida.

### 3. Asignación de puertos

Cada proceso recibe un puerto único calculado con la fórmula:

```
puerto = 50000 + (maquina - 1) * 100 + proceso
```

Ejemplos: M1P1 → 50001, M1P5 → 50005, M2P1 → 50101, M3P10 → 50210.

Esto garantiza que no haya colisiones de puertos entre procesos de distintas máquinas ni entre procesos de la misma máquina (hasta 100 procesos por máquina, rango válido según el enunciado).

### 4. Replicación de inventario y vetos

**Cuándo se replica**: Tras cada instrucción que modifica el estado:
- `VETAR` → replica la lista de vetos completa a todos los pares.
- `COMPRAR` (exitosa) → replica el inventario actualizado a todos los pares.
- `PERDONAR` → replica la lista de vetos actualizada a todos los pares.
- Counter de veto llega a 0 → se realiza un PERDONAR automático y se replica.

**Qué se replica**: Se envía siempre el **estado completo** (inventario completo o lista de vetos completa), no deltas. Esto simplifica la consistencia: el receptor simplemente reemplaza su copia.

**Cómo se replica**: Cada replicación lanza goroutines paralelas, una por par, con timeout de 3 segundos. Si un par no responde, se registra el error pero el proceso continúa normalmente.

**Condiciones de carrera**: El acceso al inventario y a la lista de vetos está protegido con `sync.RWMutex`. Las lecturas concurrentes (múltiples goroutines recibiendo actualizaciones de distintos pares) son seguras; las escrituras son exclusivas.

### 5. Recuperación de estado y quorum 2/3

Cuando un proceso se restaura tras una falla:

1. Solicita el estado a todos sus pares en paralelo mediante `SolicitarEstado`.
2. Espera **3 segundos** y acepta como máximo `3N − 1` respuestas (N = total de procesos).
3. Agrupa las respuestas por inventario idéntico (comparación JSON serializada y ordenada).
4. El grupo más grande es el candidato al quorum.
5. Si ese grupo tiene **más de 2/3** de las respuestas totales recibidas, se adopta su inventario. Los vetos se toman del mismo grupo.
6. Si no se alcanza el quorum, el proceso imprime un error crítico y **no entra al sistema** (exit code 2).

Este protocolo tolera que hasta un tercio de los procesos estén infectados o envíen datos corruptos, siempre que la mayoría honesta esté de acuerdo.

### 6. Simulación de nodos infectados

Al ejecutar `./script.sh INFECTAR`, el script envía **SIGUSR1** a todos los procesos activos en la máquina. Cada proceso captura esta señal con un goroutine dedicado y alterna su bandera `infectado`. Cuando está infectado:

- Responde a solicitudes `SolicitarEstado` con un inventario alterado (`cantidad * 2 + 1`).
- Sigue funcionando normalmente internamente (su inventario real no cambia).
- Ejecutar `INFECTAR` nuevamente desactiva la infección (comportamiento toggle).

### 7. Determinismo en la ejecución de instrucciones

Las instrucciones se leen del archivo secuencialmente. La ejecución de cada instrucción en Go es atómica desde el punto de vista del estado del proceso (protegida por mutex), y la replicación ocurre en goroutines de fondo sin bloquear la lectura del siguiente instrucción. Esto cumple el requisito de "escuchar y ejecutar instrucciones de los otros procesos de manera concurrente".

---

## Estructura del repositorio

```
tarea3/
├── cmd/
│   └── expendedora/
│       └── main.go              # Punto de entrada del proceso
├── internal/
│   ├── estado/
│   │   └── estado.go            # Estado local: inventario, vetos, infección
│   ├── grpcserver/
│   │   └── servidor.go          # Servidor gRPC (implementa el servicio)
│   ├── instrucciones/
│   │   └── ejecutor.go          # Lectura y ejecución de instrucciones
│   ├── logutil/
│   │   └── logutil.go           # Logger: consola + archivos inventario/vetos
│   └── pares/
│       └── pares.go             # Gestión de pares: descubrimiento, réplica, quorum
├── proto/
│   ├── expendedora.proto        # Definición del servicio gRPC
│   └── expendedorapb/
│       ├── expendedora.pb.go    # Tipos generados por protoc
│       └── expendedora_grpc.pb.go  # Stubs gRPC
├── inventario/
│   ├── inventario1.json         # Inventario de ejemplo 1
│   ├── inventario2.json         # Inventario de ejemplo 2
│   └── inventario3.json         # Inventario de ejemplo 3
├── instrucciones/
│   ├── proceso_1.txt            # Instrucciones para proceso con ID 1
│   ├── proceso_2.txt            # Instrucciones para proceso con ID 2
│   └── proceso_3.txt            # Instrucciones para proceso con ID 3
├── logs/                        # Generado en runtime (logs de procesos)
├── vendor/                      # Dependencias Go (compilación offline)
├── script.sh                    # Script principal de control
├── build.sh                     # Script de compilación
├── go.mod
├── go.sum
└── README.md
```

---

## Instalación y compilación

### Prerrequisitos

- Ubuntu 24.04 (máquinas virtuales asignadas)
- Go 1.22 o superior
- Python 3 (para el script bash, manejo de JSON)

### Pasos en **cada** máquina virtual

```bash
# 1. Clonar el repositorio
git clone <URL_REPOSITORIO>
cd tarea3

# 2. Instalar Go si no está instalado
sudo apt-get update && sudo apt-get install -y golang-go

# 3. Compilar el binario
./build.sh
# Esto genera el archivo ejecutable 'expendedora' en el directorio actual.

# 4. Dar permisos al script
chmod +x script.sh
```

> **Importante**: Antes de ejecutar el script en una máquina, asegurarse de que el archivo `topologia.json` esté sincronizado entre las tres máquinas. Ver sección [Uso del script](#instrucciones-de-uso-scriptsh).

---

## Instrucciones de uso (script.sh)

### Sincronización previa de topología

Antes de iniciar procesos, el archivo `topologia.json` debe existir en todas las máquinas con la misma información. El flujo recomendado es:

1. En la **máquina 1**, ejecutar el script (esto genera la topología de esa máquina).
2. Copiar el `topologia.json` resultante a las máquinas 2 y 3 (vía `scp`).
3. En las máquinas 2 y 3, ejecutar el script (que actualiza la topología con sus propios procesos).
4. Re-copiar el `topologia.json` final a las tres máquinas para que todas conozcan la topología completa.

> En la práctica, si todas las máquinas tienen el mismo número de procesos y se usa el script simultáneamente, cada máquina calculará las entradas de las otras máquinas automáticamente.

### 1. Inicializar procesos

```bash
./script.sh <NUMERO_DE_MAQUINA> <CANTIDAD_DE_PROCESOS>
```

**Ejemplo** — Iniciar 3 procesos en la máquina 1:
```bash
./script.sh 1 3
```

Esto:
- Actualiza `topologia.json` con los 3 procesos de la máquina 1.
- Inicia los procesos `M1P1`, `M1P2`, `M1P3` en segundo plano.
- Cada proceso selecciona un inventario aleatorio de `./inventario/`.
- Cada proceso busca su archivo de instrucciones en `./instrucciones/` con el formato `<nombre>_<ID>.txt`.

> Los logs de consola de cada proceso se guardan en `logs/consola_M<m>P<p>.log`.

### 2. Restaurar un proceso

```bash
./script.sh <NUMERO_DE_MAQUINA> RESTAURAR <NUMERO_DE_ID_DEL_TXT>
```

**Ejemplo** — Restaurar el proceso que lee `proceso_4.txt` en la máquina 3:
```bash
./script.sh 3 RESTAURAR 4
```

Esto:
- Mata el proceso anterior si aún existía.
- Reinicia el proceso con la flag `-restaurar`.
- El proceso solicita su estado a los pares y aplica el quorum antes de unirse al sistema.

### 3. Matar un proceso

```bash
./script.sh <NUMERO_DE_MAQUINA> MATAR <NUMERO_DE_ID_DEL_TXT>
```

**Ejemplo** — Matar el proceso que lee `proceso_4.txt` en la máquina 3:
```bash
./script.sh 3 MATAR 4
```

### 4. Matar todos los procesos de una máquina

```bash
./script.sh <NUMERO_DE_MAQUINA> KILLALL
```

**Ejemplo**:
```bash
./script.sh 2 KILLALL
```

Mata todos los procesos expendedora activos en la máquina 2 (no elimina archivos de datos).

### 5. Infectar procesos

```bash
./script.sh INFECTAR
```

Alterna el modo infección de todos los procesos activos en la máquina. Cuando están infectados, responden a solicitudes de recuperación con inventarios alterados (cantidad × 2 + 1). Ejecutar nuevamente desactiva la infección.

### 6. Ver estado de un proceso

```bash
./script.sh <NUMERO_DE_MAQUINA> ESTADO <NUMERO_DE_ID_DEL_TXT>
```

**Ejemplo**:
```bash
./script.sh 1 ESTADO 2
```

Muestra el contenido actual de los logs de inventario y vetos del proceso `M1P2`.

---

## Formatos de archivos

### Archivos de instrucciones (`instrucciones/<nombre>_<ID>.txt`)

Formato según el enunciado. Cada línea es una instrucción:

```
VETAR jack
COMPRAR jack manzana 10
COMPRAR anna dewitt manzana 15
COMPRAR anna dewitt manzana 50
COMPRAR atlas ADAM 5
```

### Inventarios (`inventario/<nombre>.json`)

```json
[
  {"nombre": "manzana", "cantidad": 100},
  {"nombre": "naranja", "cantidad": 10}
]
```

### Log de instrucciones (`logs/inventario_M<m>P<p>.log`)

```
VETAR jack
COMPRAR jack manzana 10 | DENEGADO
COMPRAR anna dewitt manzana 15 | VALIDO
COMPRAR anna dewitt manzana 50 | NO VALIDO
COMPRAR atlas ADAM 5 | VALIDO
```

### Log de vetos (`logs/vetos_M<m>P<p>.log`)

```
VETADO jack 3
VETADO sofia lamb 5
```

El counter indica las instrucciones restantes antes del perdón automático.

### Topología (`topologia.json`)

Generado automáticamente por el script:

```json
[
  {"maquina": 1, "proceso": 1, "direccion": "10.10.28.35:50001"},
  {"maquina": 1, "proceso": 2, "direccion": "10.10.28.35:50002"},
  {"maquina": 2, "proceso": 1, "direccion": "10.10.28.36:50101"},
  {"maquina": 3, "proceso": 1, "direccion": "10.10.28.37:50201"}
]
```

---

## Consideraciones especiales

### Ejecución concurrente de instrucciones

Cada proceso ejecuta sus instrucciones secuencialmente (como indica el enunciado: cada proceso tiene su propia secuencia), pero **escucha y aplica actualizaciones de otros procesos de forma concurrente** gracias al servidor gRPC que corre en una goroutine separada. El acceso al estado compartido está protegido con `sync.RWMutex`.

### Tolerancia a fallos y orden de inicio

El sistema tolera que los procesos de distintas máquinas no inicien exactamente al mismo tiempo. La ventana de espera de 2 segundos permite que los procesos se sincronicen. Si un proceso tarda más de 2 segundos en responder al registro, simplemente no se agrega como par conocido en esa sesión; podrá recuperarse usando el comando `RESTAURAR`.

### Procesos infectados y quorum

Un proceso infectado altera los inventarios que envía en respuesta a `SolicitarEstado`. Para que el quorum falle, se necesitan que **más de 1/3** de los procesos que responden estén infectados. Si solo una máquina está infectada en un sistema de 3 máquinas con el mismo número de procesos por máquina, el quorum honesto siempre ganará (2/3 honestos > 1/3 infectados).

### Uso de IA

Para el desarrollo de este proyecto se utilizó asistencia de IA (Claude, Anthropic) en la generación de la estructura inicial del código y los comentarios de documentación. Los comentarios automáticos insertados por la IA han sido revisados y adaptados al contexto del proyecto.

---

## Pruebas y ejemplos de salida

### Ejemplo 1: Inicio normal y ejecución de instrucciones

**En máquina 1** (`10.10.28.35`):
```bash
./script.sh 1 3
```

**Salida esperada en `logs/inventario_M1P1.log`**:
```
VETAR jack
COMPRAR jack manzana 10 | DENEGADO
COMPRAR anna dewitt manzana 15 | VALIDO
COMPRAR anna dewitt manzana 50 | NO VALIDO
COMPRAR atlas ADAM 5 | VALIDO
```

**Salida esperada en `logs/vetos_M1P1.log`** (al finalizar):
```
(vacío — jack fue perdonado automáticamente cuando su counter llegó a 0)
```

### Ejemplo 2: Matar y restaurar un proceso

```bash
# Matar el proceso 1 de la máquina 1
./script.sh 1 MATAR 1

# Restaurarlo
./script.sh 1 RESTAURAR 1
```

En `logs/consola_M1P1.log` se verá el proceso solicitando estado, verificando quorum y uniéndose al sistema o fallando con error crítico si no alcanza 2/3.

### Ejemplo 3: Infección y quorum fallido

```bash
# Infectar todos los procesos de la máquina 2
./script.sh INFECTAR   # (ejecutado en la máquina 2)

# Matar y restaurar un proceso en la máquina 1
./script.sh 1 MATAR 1
./script.sh 1 RESTAURAR 1
```

Si más de 1/3 de los pares están infectados, la restauración fallará con:
```
[ERROR CRITICO] M1P1: Quorum no alcanzado. El proceso no puede unirse al sistema.
```

### Ejemplo 4: Ver estado de un proceso

```bash
./script.sh 1 ESTADO 2
```

```
============================================================
 Estado de M1P2
============================================================
 Estado PID: CORRIENDO (PID 12345)

--- Inventario (logs/inventario_M1P2.log) ---
VETAR sofia lamb
COMPRAR sofia lamb agua 10 | DENEGADO
COMPRAR anna dewitt agua 20 | VALIDO
PERDONAR sofia lamb
COMPRAR sofia lamb agua 5 | VALIDO

--- Vetos (logs/vetos_M1P2.log) ---
(sin vetos activos)
============================================================
```
