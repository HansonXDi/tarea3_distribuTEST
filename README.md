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
| 1       | 10.10.28.35  | Corre una instancia de cada proceso lógico (expendedora) del sistema. Expone una API REST por cada proceso. |
| 2       | 10.10.28.36  | Igual que máquina 1: corre la réplica paralela de cada proceso lógico, comunicándose por REST con las otras dos máquinas. |
| 3       | 10.10.28.37  | Igual que máquinas 1 y 2: completa el grupo de 3 réplicas por proceso lógico, necesario para el quórum 2/3. |

No existe un nodo "maestro": las 3 máquinas son simétricas. Por cada `ProcessID` (ej. `P1`), existen 3 réplicas paralelas, una por máquina (`M1P1`, `M2P1`, `M3P1`), que ejecutan el mismo rol de forma independiente y se comparan entre sí al final.

---

## Arquitectura de comunicación: API REST

Toda la comunicación entre procesos usa **HTTP REST con JSON**. Cada proceso expone su propia API en el puerto `8000 + (machineID-1)*100 + processID`.

| Endpoint | Método | Descripción |
|----------|--------|-------------|
| `/health` | GET | Chequeo de disponibilidad, usado al iniciar para esperar a las réplicas paralelas (máx. 2s). |
| `/inventory` | POST | Una réplica paralela envía su inventario inicial recién sorteado. |
| `/state-report` | POST | Una réplica paralela envía su inventario y vetos finales para la fase de comparación por quórum. |

**Justificación:** REST sobre HTTP es ampliamente soportado por la librería estándar de Go (`net/http`), no requiere dependencias externas, y los endpoints son fáciles de probar manualmente con `curl` durante el desarrollo. Se priorizó esto sobre gRPC (más complejo de compilar/desplegar en VMs) y sobre mensajería tipo broker (innecesaria para solo 3 nodos).

| machineID | processID | Puerto | URL base |
|-----------|-----------|--------|----------|
| 1 | 1 | 8001 | http://10.10.28.35:8001 |
| 1 | 2 | 8002 | http://10.10.28.35:8002 |
| 2 | 1 | 8101 | http://10.10.28.36:8101 |
| 2 | 2 | 8102 | http://10.10.28.36:8102 |
| 3 | 1 | 8201 | http://10.10.28.37:8201 |
| 3 | 2 | 8202 | http://10.10.28.37:8202 |

---

## Ciclo de vida de un proceso (expendedora)

Cada grupo de réplicas paralelas (mismo `ProcessID` en las 3 máquinas) elige de forma determinista un **líder de inventario**: la réplica corriendo en la máquina con el `MachineID` más bajo del grupo (en una arquitectura de 3 máquinas, siempre es la réplica en la Máquina 1). Esto evita que cada réplica sortee su propio inventario de forma independiente, lo cual rompería la sincronización entre ellas.

1. **Levanta su servidor REST** propio (escucha en su puerto asignado).
2. **Si es el líder**: sortea un inventario plantilla al azar de `/inventario` y lo **copia** (nunca modifica el original) a su archivo propio. Si **no es el líder**: espera (hasta 5 segundos) a recibir ese mismo inventario desde el líder vía `POST /inventory`, y lo aplica como su copia propia. Así las 3 réplicas arrancan siempre con datos idénticos.
3. **El líder espera a sus réplicas paralelas** vía `GET /health`, hasta 2 segundos, antes de enviarles el inventario.
4. **El líder distribuye su inventario inicial** a las réplicas vía `POST /inventory`.
5. **Cada réplica ejecuta sus instrucciones** (`VETAR`/`COMPRAR`/`PERDONAR`) desde su archivo `instrucciones/proceso_<ID>.txt`, aplicándolas sobre su propia copia de inventario y vetos (ahora consistente entre las 3), generando un log por cada instrucción.
6. **Reporta su estado final** (inventario + vetos resultantes) a sus réplicas vía `POST /state-report`, y recolecta los reportes que ellas le envían.
7. **Determina el resultado por quórum**: si ≥ 2/3 de los reportes (incluido el propio) coinciden exactamente en inventario y vetos, ese es el resultado válido. Si no se alcanza el quórum, se considera que el sistema fue comprometido y se imprime:
   > *Todas las máquinas han sido infectadas, por favor revíseme.*

> **Nota:** si el líder no está disponible (caída o timeout de 5s sin recibir su inventario), una réplica cae a sortear su propio inventario como respaldo, dejando una advertencia explícita en el log, ya que esto puede introducir inconsistencias hasta que el líder vuelva a estar disponible.

---

## Modo "Infectado" (proceso bizantino)

`INFECTAR` marca **esta máquina entera** como infectada. Los procesos que se creen a continuación arrancarán en modo infectado desde el inicio: generarán datos corruptos **durante su ejecución** (vetos a personas inventadas, descuentos aleatorios de inventario, `PERDONAR` que en realidad veta), y reportarán esos datos falsos a sus réplicas en la comparación final.

**El flujo correcto es: primero `INFECTAR`, luego crear los procesos.**

```bash
# 1. Marcar la máquina como infectada (crea el flag .infectado)
./script.sh INFECTAR

# 2. Ahora crear los procesos — arrancarán infectados
./script.sh 1 2

# Para desactivar la infección (toggle):
./script.sh INFECTAR
```

`INFECTAR` crea un flag `.infectado` en el directorio de trabajo. Al ejecutar `./script.sh <M> <N>`, el script lee ese flag y crea automáticamente un flag individual por proceso (`.infectado_M<M>P<ID>`) antes de lanzarlo, garantizando el modo infectado desde el arranque sin depender del timing de señales.

---

## Estructura del repositorio

```
.
├── cmd/
│   └── main.go                  # Punto de entrada: parsea argumentos y arranca el proceso
├── internal/
│   ├── process/
│   │   └── process.go           # Ciclo de vida completo: sorteo, distribución, ejecución, quórum
│   ├── store/
│   │   └── store.go             # Inventario y vetos persistidos en disco (JSON)
│   ├── httpapi/
│   │   ├── server.go            # Servidor REST (handlers /health, /inventory, /state-report)
│   │   └── client.go            # Cliente REST para llamar a las réplicas paralelas
│   └── protocol/
│       └── types.go             # Tipos JSON compartidos entre cliente y servidor
├── instrucciones/                # Archivos proceso_<ID>.txt
├── inventario/                   # Plantillas de inventario (nunca se modifican directamente)
├── logs/                         # Logs, inventarios propios, resultados finales
├── script.sh                     # Script principal de control
├── iniciar.sh                    # Script auxiliar para ESTADO
├── go.mod
└── README.md
```

---

## Instrucciones de uso

### 1. Prerequisitos (en cada VM)

```bash
sudo apt-get update && sudo apt-get install -y golang-go
git clone <URL_PRIVADA> tarea3
cd tarea3
chmod +x script.sh iniciar.sh
```

### 2. Compilar

```bash
go build -o expendedora ./cmd/main.go
```

`script.sh` compila automáticamente si el binario no existe.

### 3. Inicializar procesos

Ejecutar en **cada máquina** (sustituir `<N_MAQUINA>` por 1, 2 o 3 según corresponda a esa VM):

```bash
# Máquina 1 (IP 10.10.28.35)
./script.sh 1 2

# Máquina 2 (IP 10.10.28.36)
./script.sh 2 2

# Máquina 3 (IP 10.10.28.37)
./script.sh 3 2
```

Cada proceso ejecuta su ciclo de vida completo automáticamente (sorteo → distribución → instrucciones → comparación final) y luego queda corriendo para seguir respondiendo a sus réplicas.

### 4. Restaurar un proceso (reiniciar su ciclo completo)

```bash
./script.sh 3 RESTAURAR 4
```

### 5. Matar un proceso específico / todos los de una máquina

```bash
./script.sh 3 MATAR 4
./script.sh 2 KILLALL
```

### 6. Activar/desactivar modo infectado

```bash
touch .infectado_M1P1   # antes de iniciar, para garantizar infección desde el arranque
./script.sh 1 2
# o, con el proceso ya corriendo:
./script.sh INFECTAR
```

### 7. Ver estado de un proceso

```bash
./iniciar.sh 1 ESTADO 1
```

Muestra el inventario propio (JSON persistido), los vetos actuales, y el resultado de la última comparación por quórum, si existe.

---

## Formato de archivos de instrucciones

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
COMPRAR atlas ADAM 5 | VALIDO
```

**`logs/vetos_M<M>P<P>.log`** – estado final de vetos:

```
VETADO jack 3
```

**`logs/inventario_propio_M<M>P<P>.json`** – el inventario propio del proceso (copia individual, nunca el archivo plantilla original).

**`logs/resultado_M<M>P<P>.txt`** – resultado de la comparación final por quórum: o bien el inventario/vetos validados, o el mensaje de error de integridad.

---

## Mecanismos de consistencia

| Problema | Solución implementada |
|----------|-----------------------|
| Cada proceso debe tener su copia individual de inventario | `Store.LoadFromTemplate` copia (no referencia) el JSON elegido al azar hacia un archivo propio del proceso |
| Las 3 réplicas de un mismo proceso deben arrancar con el MISMO inventario | Solo la réplica de menor `MachineID` ("líder") sortea el inventario; las demás lo reciben vía `POST /inventory` y lo aplican como propio, evitando sorteos independientes que romperían la sincronización |
| Verificar integridad entre réplicas paralelas | Comparación final por quórum 2/3 vía `POST /state-report`; si no se alcanza, se reporta el error de integridad |
| Detección de comportamiento bizantino | El modo "infectado" simula un nodo que reporta datos falsos; si suficientes nodos están infectados, el quórum falla y el sistema lo detecta |
| Condiciones de carrera en el Store local | `sync.RWMutex` en `Store`; todas las operaciones (compra, veto, perdón) son atómicas |
| Eliminación selectiva de un proceso | Cada expendedora corre como **proceso real e independiente del SO** (un binario por proceso vía `PROC <ID>`), por lo que `MATAR <ID>` solo afecta a ese proceso |

---

## Uso de IA

Se utilizó asistencia de IA (Claude) para la generación inicial de la estructura de los archivos Go y el script bash, incluyendo el rediseño de la arquitectura hacia API REST y el mecanismo de quórum/infección. Todo el código fue revisado, adaptado y documentado manualmente por el grupo. Los comentarios automáticos fueron eliminados y reemplazados por los presentes en el código.

---

## Consideraciones especiales

- Cada proceso, al reportar su estado final, espera hasta 3 segundos a recibir los reportes de sus réplicas paralelas antes de resolver el quórum con los datos disponibles.
- Si una réplica paralela no responde a tiempo (caída, red lenta), el quórum se calcula solo con los reportes disponibles; con 3 réplicas totales, perder una reduce el quórum a comparar 1 vs 1, lo cual nunca alcanza 2/3 salvo coincidencia exacta de ambos.
- El archivo flag de infección (`.infectado_M<M>P<P>`) vive en el directorio de trabajo del proceso; asegúrese de ejecutar `script.sh` siempre desde la raíz del repositorio.
- Los puertos `8001`–`8399` deben estar abiertos entre las 3 VMs para que la API REST funcione.
