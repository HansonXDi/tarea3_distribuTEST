#!/usr/bin/env bash
# =============================================================================
# script.sh - Script principal de control del sistema de expendedoras
# =============================================================================
# Uso:
#   ./script.sh <NUMERO_DE_MAQUINA> <CANTIDAD_DE_PROCESOS>   -> Inicialización
#   ./script.sh <NUMERO_DE_MAQUINA> RESTAURAR <ID>           -> Restaurar proceso
#   ./script.sh <NUMERO_DE_MAQUINA> MATAR <ID>               -> Matar proceso
#   ./script.sh <NUMERO_DE_MAQUINA> KILLALL                  -> Matar todos los procesos
#   ./script.sh INFECTAR                                     -> Toggle infección
#   ./script.sh <NUMERO_DE_MAQUINA> ESTADO <ID>              -> Ver estado de proceso
# =============================================================================

set -euo pipefail

# --- Configuración ---
BINARIO="./expendedora"                  # Ruta al binario compilado
TOPOLOGIA_JSON="topologia.json"          # Archivo de topología compartido
PIDS_DIR=".pids"                         # Directorio para guardar PIDs
LOG_DIR="logs"                           # Directorio de logs
INFECTADO_FLAG=".infectado"              # Flag de infección activa

# IPs de las máquinas virtuales
declare -A IPS_MAQUINAS
IPS_MAQUINAS[1]="10.10.28.35"
IPS_MAQUINAS[2]="10.10.28.36"
IPS_MAQUINAS[3]="10.10.28.37"

# Puerto base: puerto = 50000 + (maquina-1)*100 + proceso
calcular_puerto() {
    local maquina=$1
    local proceso=$2
    echo $((50000 + (maquina - 1) * 100 + proceso))
}

# Archivo PID de un proceso específico
archivo_pid() {
    local maquina=$1
    local proceso=$2
    echo "${PIDS_DIR}/M${maquina}P${proceso}.pid"
}

# Verificar que el binario existe
verificar_binario() {
    if [ ! -f "$BINARIO" ]; then
        echo "[ERROR] No se encontró el binario '$BINARIO'."
        echo "        Compila primero con: go build -o expendedora ./cmd/expendedora"
        exit 1
    fi
}

# Verificar que la carpeta instrucciones tiene archivos suficientes
verificar_instrucciones() {
    local cantidad=$1
    local disponibles
    disponibles=$(find instrucciones -name "*.txt" 2>/dev/null | wc -l)
    if [ "$disponibles" -lt "$cantidad" ]; then
        echo "[WARN] Solo hay $disponibles archivos de instrucciones, se pedían $cantidad."
    fi
}

# =============================================================================
# INICIALIZACIÓN: ./script.sh <MAQUINA> <CANTIDAD>
# =============================================================================
inicializar() {
    local maquina=$1
    local cantidad=$2

    verificar_binario
    verificar_instrucciones "$cantidad"
    mkdir -p "$PIDS_DIR" "$LOG_DIR" instrucciones inventario

    echo "============================================================"
    echo " Inicializando $cantidad proceso(s) en máquina $maquina"
    echo "============================================================"

    # --- Construir/actualizar topología ---
    # La topología incluye todos los procesos que se ejecutarán en ESTA máquina.
    # Se asume que las otras máquinas tienen la misma cantidad de procesos.
    actualizar_topologia "$maquina" "$cantidad"

    # --- Iniciar procesos ---
    for (( i=1; i<=cantidad; i++ )); do
        iniciar_proceso "$maquina" "$i" false
    done

    echo ""
    echo "[OK] $cantidad expendedora(s) iniciadas en máquina $maquina."
    echo "     Usa './script.sh $maquina ESTADO <ID>' para ver el estado de un proceso."
}

# Agrega las entradas de esta máquina al archivo de topología.
# El archivo topologia.json contiene todos los procesos del sistema.
actualizar_topologia() {
    local maquina=$1
    local cantidad=$2

    # Leer entradas existentes de OTRAS máquinas
    local entradas_existentes="[]"
    if [ -f "$TOPOLOGIA_JSON" ]; then
        # Filtrar entradas que NO sean de esta máquina
        entradas_existentes=$(python3 -c "
import json, sys
with open('$TOPOLOGIA_JSON') as f:
    data = json.load(f)
data = [e for e in data if e['maquina'] != $maquina]
print(json.dumps(data))
" 2>/dev/null || echo "[]")
    fi

    # Construir entradas de esta máquina
    local nuevas_entradas="[]"
    for (( i=1; i<=cantidad; i++ )); do
        local ip="${IPS_MAQUINAS[$maquina]}"
        local puerto
        puerto=$(calcular_puerto "$maquina" "$i")
        nuevas_entradas=$(python3 -c "
import json
existing = $entradas_existentes
new_entry = {'maquina': $maquina, 'proceso': $i, 'direccion': '${ip}:${puerto}'}
existing.append(new_entry)
print(json.dumps(existing, indent=2))
")
        entradas_existentes="$nuevas_entradas"
    done

    echo "$entradas_existentes" > "$TOPOLOGIA_JSON"
    echo "[topologia] Actualizada con $cantidad proceso(s) de máquina $maquina"
}

# Inicia un proceso expendedora en segundo plano.
iniciar_proceso() {
    local maquina=$1
    local proceso=$2
    local restaurar=$3

    local pid_file
    pid_file=$(archivo_pid "$maquina" "$proceso")

    # Verificar si ya existe y está corriendo
    if [ -f "$pid_file" ]; then
        local pid_existente
        pid_existente=$(cat "$pid_file")
        if kill -0 "$pid_existente" 2>/dev/null; then
            echo "[WARN] M${maquina}P${proceso} ya está corriendo (PID $pid_existente)"
            return
        fi
    fi

    local args="-maquina $maquina -proceso $proceso"
    if [ "$restaurar" = "true" ]; then
        args="$args -restaurar"
    fi

    # Iniciar en segundo plano, redirigiendo stdout/stderr a archivo de log de consola
    local log_consola="${LOG_DIR}/consola_M${maquina}P${proceso}.log"
    # shellcheck disable=SC2086
    nohup $BINARIO $args > "$log_consola" 2>&1 &
    local pid=$!
    echo "$pid" > "$pid_file"

    echo "[OK] M${maquina}P${proceso} iniciado (PID $pid, log: $log_consola)"
}

# =============================================================================
# RESTAURAR: ./script.sh <MAQUINA> RESTAURAR <ID>
# =============================================================================
restaurar() {
    local maquina=$1
    local proceso_id=$2

    verificar_binario
    echo "[restaurar] Restaurando proceso $proceso_id en máquina $maquina..."

    # Matar el proceso anterior si existe
    matar_proceso "$maquina" "$proceso_id" 2>/dev/null || true

    sleep 1
    iniciar_proceso "$maquina" "$proceso_id" true
    echo "[OK] Proceso M${maquina}P${proceso_id} restaurado."
}

# =============================================================================
# MATAR: ./script.sh <MAQUINA> MATAR <ID>
# =============================================================================
matar() {
    local maquina=$1
    local proceso_id=$2

    echo "[matar] Matando proceso $proceso_id en máquina $maquina..."
    matar_proceso "$maquina" "$proceso_id"
    echo "[OK] Proceso M${maquina}P${proceso_id} eliminado."
}

# Mata un proceso individual dado su ID.
matar_proceso() {
    local maquina=$1
    local proceso=$2

    local pid_file
    pid_file=$(archivo_pid "$maquina" "$proceso")

    if [ ! -f "$pid_file" ]; then
        echo "[WARN] No se encontró PID para M${maquina}P${proceso}"
        return 1
    fi

    local pid
    pid=$(cat "$pid_file")

    if kill -0 "$pid" 2>/dev/null; then
        kill -TERM "$pid" 2>/dev/null || true
        sleep 0.5
        # Si sigue corriendo, forzar
        if kill -0 "$pid" 2>/dev/null; then
            kill -KILL "$pid" 2>/dev/null || true
        fi
        echo "[matar] Proceso M${maquina}P${proceso} (PID $pid) terminado."
    else
        echo "[WARN] El proceso M${maquina}P${proceso} (PID $pid) no estaba corriendo."
    fi

    rm -f "$pid_file"
}

# =============================================================================
# KILLALL: ./script.sh <MAQUINA> KILLALL
# =============================================================================
killall_maquina() {
    local maquina=$1

    echo "[killall] Matando todos los procesos de máquina $maquina..."
    local count=0

    # Buscar todos los PIDs de esta máquina
    for pid_file in "${PIDS_DIR}"/M${maquina}P*.pid; do
        if [ -f "$pid_file" ]; then
            local nombre
            nombre=$(basename "$pid_file" .pid)
            local proceso_num="${nombre#M${maquina}P}"
            matar_proceso "$maquina" "$proceso_num" 2>/dev/null || true
            ((count++)) || true
        fi
    done

    if [ "$count" -eq 0 ]; then
        echo "[WARN] No se encontraron procesos activos en máquina $maquina"
    else
        echo "[OK] $count proceso(s) de máquina $maquina eliminados."
    fi
}

# =============================================================================
# INFECTAR: ./script.sh INFECTAR
# =============================================================================
infectar() {
    echo "[infectar] Alternando modo infección en todos los procesos de esta máquina..."

    # Enviar señal SIGUSR1 a todos los procesos expendedora activos
    # Los procesos capturan esta señal y alternan su estado de infección
    local count=0
    for pid_file in "${PIDS_DIR}"/M*.pid; do
        if [ -f "$pid_file" ]; then
            local pid
            pid=$(cat "$pid_file")
            if kill -0 "$pid" 2>/dev/null; then
                kill -SIGUSR1 "$pid" 2>/dev/null && ((count++)) || true
            fi
        fi
    done

    if [ -f "$INFECTADO_FLAG" ]; then
        rm -f "$INFECTADO_FLAG"
        echo "[infectar] Modo infección DESACTIVADO ($count proceso(s) afectados)."
    else
        touch "$INFECTADO_FLAG"
        echo "[infectar] Modo infección ACTIVADO ($count proceso(s) afectados)."
        echo "           Los procesos enviarán inventarios alterados a los pares."
    fi
}

# =============================================================================
# ESTADO: ./script.sh <MAQUINA> ESTADO <ID>
# =============================================================================
mostrar_estado() {
    local maquina=$1
    local proceso_id=$2

    echo "============================================================"
    echo " Estado de M${maquina}P${proceso_id}"
    echo "============================================================"

    local pid_file
    pid_file=$(archivo_pid "$maquina" "$proceso_id")
    if [ -f "$pid_file" ]; then
        local pid
        pid=$(cat "$pid_file")
        if kill -0 "$pid" 2>/dev/null; then
            echo " Estado PID: CORRIENDO (PID $pid)"
        else
            echo " Estado PID: DETENIDO"
        fi
    else
        echo " Estado PID: NO INICIADO"
    fi

    echo ""
    echo "--- Inventario (logs/inventario_M${maquina}P${proceso_id}.log) ---"
    local log_inv="${LOG_DIR}/inventario_M${maquina}P${proceso_id}.log"
    if [ -f "$log_inv" ]; then
        cat "$log_inv"
    else
        echo "(sin log de inventario)"
    fi

    echo ""
    echo "--- Vetos (logs/vetos_M${maquina}P${proceso_id}.log) ---"
    local log_vetos="${LOG_DIR}/vetos_M${maquina}P${proceso_id}.log"
    if [ -f "$log_vetos" ]; then
        if [ -s "$log_vetos" ]; then
            cat "$log_vetos"
        else
            echo "(sin vetos activos)"
        fi
    else
        echo "(sin log de vetos)"
    fi
    echo "============================================================"
}

# =============================================================================
# PUNTO DE ENTRADA - Parseo de argumentos
# =============================================================================
if [ $# -eq 0 ]; then
    echo "Uso:"
    echo "  $0 <MAQUINA> <CANTIDAD>          - Inicializar procesos"
    echo "  $0 <MAQUINA> RESTAURAR <ID>      - Restaurar proceso"
    echo "  $0 <MAQUINA> MATAR <ID>          - Matar proceso"
    echo "  $0 <MAQUINA> KILLALL             - Matar todos los procesos"
    echo "  $0 INFECTAR                       - Toggle infección"
    echo "  $0 <MAQUINA> ESTADO <ID>         - Ver estado de proceso"
    exit 0
fi

# Caso especial: INFECTAR no lleva número de máquina
if [ "$1" = "INFECTAR" ]; then
    infectar
    exit 0
fi

# Todos los demás casos requieren al menos 2 argumentos
if [ $# -lt 2 ]; then
    echo "[ERROR] Se requieren al menos 2 argumentos."
    exit 1
fi

MAQUINA="$1"
COMANDO="$2"

# Validar número de máquina
if ! [[ "$MAQUINA" =~ ^[1-3]$ ]]; then
    echo "[ERROR] NUMERO_DE_MAQUINA debe ser 1, 2 o 3. Recibido: $MAQUINA"
    exit 1
fi

case "$COMANDO" in
    RESTAURAR)
        if [ $# -lt 3 ]; then
            echo "[ERROR] RESTAURAR requiere: $0 <MAQUINA> RESTAURAR <ID>"
            exit 1
        fi
        restaurar "$MAQUINA" "$3"
        ;;
    MATAR)
        if [ $# -lt 3 ]; then
            echo "[ERROR] MATAR requiere: $0 <MAQUINA> MATAR <ID>"
            exit 1
        fi
        matar "$MAQUINA" "$3"
        ;;
    KILLALL)
        killall_maquina "$MAQUINA"
        ;;
    ESTADO)
        if [ $# -lt 3 ]; then
            echo "[ERROR] ESTADO requiere: $0 <MAQUINA> ESTADO <ID>"
            exit 1
        fi
        mostrar_estado "$MAQUINA" "$3"
        ;;
    [0-9]*)
        # Es un número -> inicialización
        inicializar "$MAQUINA" "$COMANDO"
        ;;
    *)
        echo "[ERROR] Comando desconocido: $COMANDO"
        echo "Comandos válidos: RESTAURAR, MATAR, KILLALL, INFECTAR, ESTADO, o un número para inicializar"
        exit 1
        ;;
esac
