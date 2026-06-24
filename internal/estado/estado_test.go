package estado

import (
	"testing"
)

func TestVetar(t *testing.T) {
	p := NuevoProceso(1, 1, []Item{{Nombre: "manzana", Cantidad: 100}})

	// Vetar por primera vez
	esNuevo := p.VetarPersona("jack")
	if !esNuevo {
		t.Error("Esperaba veto nuevo")
	}
	counter, ok := p.CounterVeto("jack")
	if !ok || counter != 5 {
		t.Errorf("Counter esperado 5, obtenido %d", counter)
	}

	// Renovar veto
	esNuevo = p.VetarPersona("jack")
	if esNuevo {
		t.Error("No debería ser nuevo (renovación)")
	}
	counter, _ = p.CounterVeto("jack")
	if counter != 5 {
		t.Errorf("Counter esperado 5 tras renovación, obtenido %d", counter)
	}
}

func TestCompraVetado(t *testing.T) {
	p := NuevoProceso(1, 1, []Item{{Nombre: "manzana", Cantidad: 100}})
	p.VetarPersona("jack")

	resultado := p.IntentarCompra("jack", "manzana", 10)
	if resultado != "DENEGADO" {
		t.Errorf("Esperaba DENEGADO, obtuvo %s", resultado)
	}
}

func TestCompraExitosa(t *testing.T) {
	p := NuevoProceso(1, 1, []Item{{Nombre: "manzana", Cantidad: 100}})
	resultado := p.IntentarCompra("anna", "manzana", 15)
	if resultado != "VALIDO" {
		t.Errorf("Esperaba VALIDO, obtuvo %s", resultado)
	}
	items := p.ObtenerInventario()
	if items[0].Cantidad != 85 {
		t.Errorf("Esperaba cantidad 85, obtuvo %d", items[0].Cantidad)
	}
}

func TestCompraStockInsuficiente(t *testing.T) {
	p := NuevoProceso(1, 1, []Item{{Nombre: "manzana", Cantidad: 10}})
	resultado := p.IntentarCompra("anna", "manzana", 50)
	if resultado != "NO VALIDO" {
		t.Errorf("Esperaba NO VALIDO, obtuvo %s", resultado)
	}
}

func TestDecrementarCounter(t *testing.T) {
	p := NuevoProceso(1, 1, nil)
	p.VetarPersona("jack") // counter = 5

	for i := 0; i < 4; i++ {
		perdonados := p.DecrementarCounterVetos()
		if len(perdonados) != 0 {
			t.Errorf("No debería haber perdonados en iteración %d", i)
		}
	}

	// En la 5ta instrucción el counter llega a 0
	perdonados := p.DecrementarCounterVetos()
	if len(perdonados) != 1 || perdonados[0] != "jack" {
		t.Errorf("Esperaba perdonar a 'jack', obtuvo: %v", perdonados)
	}
	_, aun := p.CounterVeto("jack")
	if aun {
		t.Error("jack no debería estar vetado")
	}
}

func TestInfeccion(t *testing.T) {
	p := NuevoProceso(1, 1, []Item{{Nombre: "manzana", Cantidad: 10}})

	// No infectado: inventario normal
	items := p.InventarioParaEnvio()
	if items[0].Cantidad != 10 {
		t.Error("Esperaba cantidad normal 10")
	}

	// Infectar
	p.ToggleInfeccion()
	itemsInf := p.InventarioParaEnvio()
	if itemsInf[0].Cantidad != 21 { // 10*2+1
		t.Errorf("Esperaba cantidad alterada 21, obtuvo %d", itemsInf[0].Cantidad)
	}

	// Desinfectar
	p.ToggleInfeccion()
	itemsNorm := p.InventarioParaEnvio()
	if itemsNorm[0].Cantidad != 10 {
		t.Error("Esperaba cantidad normal 10 tras desinfección")
	}
}
