#!/usr/bin/env bash
# script.sh - Control de procesos expendedoras para Tarea 3 INF-343
# Uso:
#   ./script.sh <MAQUINA> <CANTIDAD>          -> iniciar N procesos
#   ./script.sh <MAQUINA> RESTAURAR <ID>      -> restaurar proceso
#   ./script.sh <MAQUINA> MATAR <ID>          -> matar proceso por ID
#   ./script.sh <MAQUINA> KILLALL             -> matar todos los procesos de la máquina
#   ./script.sh INFECTAR                      -> togglear modo malicioso en todos los procesos
#   ./script.sh <MAQUINA> ESTADO <ID>         -> ver estado de un proceso (ver iniciar.sh)

set -euo pipefail

BINARY="./expendedora"
PIDS_DIR=".pids"
MALICIOUS_FLAG=".malicious"

mkdir -p "$PIDS_DIR" logs

# build_binary: compila el binario Go si no existe o si los fuentes cambiaron.
# Entrada: ninguna. Salida: ninguna (falla con exit 1 si no compila).
build_binary() {
    echo "[script] Compilando..."
    go build -o "$BINARY" ./cmd/main.go
    echo "[script] Compilación exitosa."
}

# pid_file: retorna el path del archivo PID para una máquina y proceso dado.
# Entrada: MAQUINA, ID. Salida: path string.
pid_file() {
    echo "$PIDS_DIR/m${1}p${2}.pid"
}

# start_processes: lanza N procesos como N binarios independientes (un proceso
# real del SO por cada expendedora), permitiendo matar/restaurar cada uno por
# separado sin afectar a los demás.
# Entrada: MAQUINA, N. Salida: ninguna.
start_processes() {
    local MAQUINA=$1
    local N=$2
    [ -f "$BINARY" ] || build_binary

    for ID in $(seq 1 "$N"); do
        local PFILE
        PFILE=$(pid_file "$MAQUINA" "$ID")
        if [ -f "$PFILE" ] && kill -0 "$(cat "$PFILE")" 2>/dev/null; then
            echo "[script] Proceso M${MAQUINA}P${ID} ya está corriendo (PID=$(cat "$PFILE"))."
            continue
        fi
        "$BINARY" "$MAQUINA" PROC "$ID" "$N" &
        echo $! > "$PFILE"
        echo "$N" > "${PFILE}.n"
        echo "[script] Iniciado M${MAQUINA}P${ID} (PID=$!)."
    done
}

# restore_process: restaura un proceso específico, ejecutando el protocolo de recuperación.
# Entrada: MAQUINA, ID. Salida: ninguna.
restore_process() {
    local MAQUINA=$1
    local ID=$2
    [ -f "$BINARY" ] || build_binary
    local PFILE
    PFILE=$(pid_file "$MAQUINA" "$ID")

    # Recuperar N total guardado al iniciar; si no existe, asumir 2 por defecto.
    local N=2
    if [ -f "${PFILE}.n" ]; then
        N=$(cat "${PFILE}.n")
    fi

    "$BINARY" "$MAQUINA" RESTAURAR "$ID" "$N" &
    echo $! > "$PFILE"
    echo "$N" > "${PFILE}.n"
    echo "[script] Proceso M${MAQUINA}P${ID} restaurado (PID=$!)."
}

# kill_process: termina un proceso específico por su PID guardado.
# Entrada: MAQUINA, ID. Salida: ninguna.
kill_process() {
    local MAQUINA=$1
    local ID=$2
    local PFILE
    PFILE=$(pid_file "$MAQUINA" "$ID")
    if [ -f "$PFILE" ]; then
        local PID
        PID=$(cat "$PFILE")
        if kill -9 "$PID" 2>/dev/null; then
            echo "[script] Proceso M${MAQUINA}P${ID} (PID=$PID) terminado."
        else
            echo "[script] PID $PID no encontrado (quizás ya terminó)."
        fi
        rm -f "$PFILE" "${PFILE}.n"
    else
        echo "[script] No hay PID registrado para M${MAQUINA}P${ID}."
    fi
}

# killall_machine: termina todos los procesos de una máquina.
# Entrada: MAQUINA. Salida: ninguna.
killall_machine() {
    local MAQUINA=$1
    for PFILE in "$PIDS_DIR"/m${MAQUINA}p*.pid; do
        [ -f "$PFILE" ] || continue
        local PID
        PID=$(cat "$PFILE")
        kill -9 "$PID" 2>/dev/null && echo "[script] Terminado PID=$PID" || true
        rm -f "$PFILE" "${PFILE}.n"
    done
    echo "[script] Todos los procesos de la máquina $MAQUINA terminados."
}

# toggle_malicious: crea o elimina el flag de modo malicioso y envía SIGUSR1 a todos los procesos activos.
# Entrada: ninguna. Salida: ninguna.
toggle_malicious() {
    if [ -f "$MALICIOUS_FLAG" ]; then
        rm -f "$MALICIOUS_FLAG"
        echo "[script] Modo malicioso DESACTIVADO."
    else
        touch "$MALICIOUS_FLAG"
        echo "[script] Modo malicioso ACTIVADO."
    fi
    # Notificar a todos los procesos activos (SIGUSR1 = toggle malicioso)
    for PFILE in "$PIDS_DIR"/*.pid; do
        [ -f "$PFILE" ] || continue
        local PID
        PID=$(cat "$PFILE")
        kill -USR1 "$PID" 2>/dev/null || true
    done
}

# --- Parsing de argumentos ---

if [ $# -eq 0 ]; then
    echo "Uso: ./script.sh <MAQUINA> <CANTIDAD|RESTAURAR|MATAR|KILLALL|ESTADO> [ID]"
    echo "     ./script.sh INFECTAR"
    exit 1
fi

CMD="${1^^}"   # convertir a mayúsculas

case "$CMD" in
    INFECTAR)
        toggle_malicious
        ;;
    [0-9]*)
        MAQUINA=$1
        if [ $# -lt 2 ]; then
            echo "Error: falta subcomando."
            exit 1
        fi
        SUBCMD="${2^^}"
        case "$SUBCMD" in
            RESTAURAR)
                [ $# -ge 3 ] || { echo "Error: falta ID."; exit 1; }
                restore_process "$MAQUINA" "$3"
                ;;
            MATAR)
                [ $# -ge 3 ] || { echo "Error: falta ID."; exit 1; }
                kill_process "$MAQUINA" "$3"
                ;;
            KILLALL)
                killall_machine "$MAQUINA"
                ;;
            [0-9]*)
                # ./script.sh <MAQUINA> <N>
                start_processes "$MAQUINA" "$SUBCMD"
                ;;
            *)
                echo "Subcomando desconocido: $SUBCMD"
                exit 1
                ;;
        esac
        ;;
    *)
        echo "Argumento desconocido: $CMD"
        exit 1
        ;;
esac
