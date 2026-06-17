# Tarea 3 – INF-343 Sistemas Distribuidos: ¿Y El Pensador?

## Integrantes

| Nombre | Apellido | Rol |
|--------|----------|-----|
| — | — | — |
| — | — | — |
| — | — | — |

> Completar con los datos de cada integrante.

---

## Rol de cada máquina virtual

| Máquina | IP           | Rol |
|---------|--------------|-----|
| 1       | 10.10.28.35  | Nodo primario. Es la primera en levantar sus procesos; los demás esperan que esté disponible. |
| 2       | 10.10.28.36  | Nodo secundario. Replica el estado de máquina 1 y puede ejecutar instrucciones de forma concurrente. |
| 3       | 10.10.28.37  | Nodo terciario. Igual a máquina 2; participa en el quórum de recuperación (2/3 de votos). |

Cada máquina corre los mismos procesos Go (`expendedora`). No existe un nodo "maestro" permanente: la consistencia se logra por replicación activa y votación por mayoría.

---

## Arquitectura de comunicación

Se utiliza **TCP con mensajes JSON** (una línea por mensaje).  
Cada proceso escucha en el puerto `8000 + (machineID-1)*100 + processID`.

| machineID | processID | Puerto |
|-----------|-----------|--------|
| 1 | 1 | 8001 |
| 1 | 2 | 8002 |
| 2 | 1 | 8101 |
| 2 | 2 | 8102 |
| 3 | 1 | 8201 |
| 3 | 2 | 8202 |

**Justificación:** TCP garantiza entrega ordenada sin necesitar infraestructura de broker (a diferencia de AMQP/Kafka). JSON hace los mensajes legibles e inspeccionables durante desarrollo. gRPC fue descartado por añadir complejidad de compilación en las VMs.

---

## Estructura del repositorio

```
.
├── cmd/
│   └── main.go              # Punto de entrada del binario
├── internal/
│   ├── process/
│   │   └── process.go       # Lógica de una expendedora
│   ├── store/
│   │   └── store.go         # Estado local (inventario + vetos), thread-safe
│   ├── sync/
│   │   └── network.go       # Capa de red TCP, broadcast, modo malicioso
│   └── protocol/
│       └── types.go         # Tipos de mensaje compartidos
├── instrucciones/           # Archivos proceso_<ID>.txt
├── inventario/              # Archivos inventario*.json
├── logs/                    # Logs generados en ejecución
├── script.sh                # Script principal de control
├── iniciar.sh               # Script auxiliar para ESTADO
├── go.mod
└── README.md
```

---

## Instrucciones de uso

### 1. Prerequisitos (en cada VM)

```bash
# Instalar Go 1.22+
sudo apt-get update && sudo apt-get install -y golang-go

# Clonar el repositorio
git clone <URL_PRIVADA> tarea3
cd tarea3
chmod +x script.sh iniciar.sh
```

### 2. Compilar

```bash
go build -o expendedora ./cmd/main.go
```

O el propio `script.sh` compila automáticamente si no existe el binario.

### 3. Inicializar procesos

Ejecutar en **cada máquina** (sustituir `<N_MAQUINA>` por 1, 2 o 3):

```bash
# Ejemplo: levantar 2 procesos en la máquina 1
./script.sh 1 2

# Máquina 2
./script.sh 2 2

# Máquina 3
./script.sh 3 2
```

> Los procesos esperan hasta 2 segundos a que los peers se conecten antes de ejecutar instrucciones.

### 4. Restaurar un proceso caído

```bash
# Restaura el proceso que lee proceso_4.txt en la máquina 3
./script.sh 3 RESTAURAR 4
```

El proceso espera 3 segundos recibiendo inventarios de los peers, elige el que tiene ≥ 2/3 de réplicas iguales y reanuda la ejecución.

### 5. Matar un proceso específico

```bash
./script.sh 3 MATAR 4
```

### 6. Matar todos los procesos de una máquina

```bash
./script.sh 2 KILLALL
```

### 7. Infectar / desinfectar procesos (toggle)

Ejecutar desde la máquina que se quiere infectar:

```bash
./script.sh INFECTAR
```

Volver a ejecutar lo desactiva. Los procesos activos reciben `SIGUSR1` para cambiar de estado.

### 8. Ver estado de un proceso

```bash
./iniciar.sh 1 ESTADO 1
```

---

## Formato de archivos de instrucciones

Los archivos deben estar en `instrucciones/` con el nombre `<nombre>_<ID>.txt`:

```
instrucciones/proceso_1.txt
instrucciones/proceso_2.txt
```

Instrucciones soportadas:

```
VETAR <nombre>
COMPRAR <persona> <producto> <cantidad>
PERDONAR <nombre>
```

---

## Formato de logs generados

**`logs/inventario_M<M>P<P>.log`** – una línea por instrucción ejecutada:

```
VETAR jack
COMPRAR jack manzana 10 | DENEGADO
COMPRAR anna dewitt manzana 15 | NO VALIDO
COMPRAR atlas ADAM 5 | VALIDO
```

**`logs/vetos_M<M>P<P>.log`** – estado actual de vetos (se sobreescribe):

```
VETADO jack 3
VETADO sofia lamb 5
```

---

## Mecanismos de consistencia

| Problema | Solución implementada |
|----------|-----------------------|
| Pérdida de estado al apagar | Inventario y vetos replicados en todos los nodos vía broadcast TCP |
| Inventarios corruptos en recuperación | Votación por mayoría: se acepta el inventario con ≥ 2/3 réplicas iguales |
| Condiciones de carrera en vetos | `sync.RWMutex` en `Store`; actualizaciones propagadas atómicamente |
| Inventarios corruptos (modo malicioso) | Los mensajes de tipo `MALICIOUS_INV` son detectados y descartados |
| Eliminación selectiva de un proceso | Cada expendedora corre como **proceso real e independiente del SO** (un binario por proceso), no como goroutine compartida; así `MATAR <ID>` solo afecta a ese proceso |

> **Nota de diseño:** `script.sh` lanza un binario `./expendedora <MAQUINA> PROC <ID> <N>` por cada proceso lógico. Esto permite que `MATAR`/`KILLALL`/`RESTAURAR` operen sobre procesos reales del sistema operativo sin afectar a los demás. El comando `ESTADO` lee el inventario y vetos directamente desde `logs/snapshot_M<M>P<P>.json` y `logs/vetos_M<M>P<P>.log` (escritos por el proceso en ejecución), evitando crear una instancia nueva que choque con el puerto TCP ya ocupado.

---

## Uso de IA

Se utilizó asistencia de IA (Claude) para la generación inicial de la estructura de los archivos Go y el script bash. Todo el código fue revisado, adaptado y documentado manualmente por el grupo. Los comentarios automáticos fueron eliminados y reemplazados por los presentes en el código.

---

## Consideraciones especiales

- Se asume que `CANTIDAD_DE_PROCESOS` es igual en las 3 máquinas.
- El puerto de escucha se calcula automáticamente; asegurarse de que las VMs tengan abiertos los puertos `8001`–`8399`.
- Si una VM no tiene `go` instalado, el script llama `go build` que fallará; instalar Go primero.
- Los archivos de `instrucciones/` deben existir antes de ejecutar; el sistema no los genera.
