# Guía de despliegue — Oficina contable

Guía breve para instalar el **Agente InvoicesUp** en el ordenador de la oficina.
El agente trae automáticamente las facturas desde InvoicesUp y las deja como
archivos que **Golden** importa.

## Qué necesitas antes de empezar

- Windows 10/11 (o Windows Server) con permisos de **administrador**.
- Estos tres datos, que te facilita el administrador de InvoicesUp:
  1. **URL** de InvoicesUp (p. ej. `https://invoicesup.kordino.com`).
  2. **Token de conector** (una clave larga; se genera en InvoicesUp → Usuarios
     → editar el usuario de la oficina → *Generar token*).
  3. **Carpeta local** donde quieres que aparezcan los archivos (p. ej.
     `C:\InvoicesUp\exports`). Es una carpeta del disco, no una unidad de red.

## Instalación

1. Ejecuta **`invoicesup-agent-setup.exe`** (clic con el botón derecho →
   *Ejecutar como administrador*).
2. En la pantalla *Configuración del agente*, rellena los tres datos anteriores.
3. Finaliza el asistente. Al terminar, el agente queda **instalado como servicio
   de Windows** y se inicia solo; no hay que abrir ninguna ventana.

A partir de ahí se inicia con el equipo y se ejecuta en segundo plano.

## Comprobar que funciona

- Abre **Servicios** (`services.msc`) y busca **«InvoicesUp Connector Agent»**:
  debe estar en estado **En ejecución**.
- Cuando el administrador exporte facturas desde InvoicesUp, en unos ~30 segundos
  van apareciendo archivos `CODIGO_facturas.txt` en tu carpeta (uno por empresa).

## Configurar Golden

- En Golden, apunta la importación a tu carpeta local (p. ej.
  `C:\InvoicesUp\exports\SPM_facturas.txt`) — **un archivo por empresa**, según el
  código de cada una.
- Activa en Golden la opción de **eliminar el archivo tras importar**. El agente
  detecta ese borrado, avisa a InvoicesUp de que ya lo has importado y trae las
  siguientes facturas nuevas. Así el ciclo se repite solo.

## Si algo no funciona

- **El servicio no se inicia / se detiene solo** → consulta el registro en
  `C:\ProgramData\InvoicesUp\agent.log`. Suele ser un dato mal introducido (URL,
  token o carpeta). Reinstala corrigiendo el dato.
- **No aparecen archivos** → confirma con el administrador que (a) el token es el
  correcto y sigue activo y (b) que la oficina tiene empresas asignadas y hay
  facturas exportadas. Si no hay facturas nuevas, no se escribe ningún archivo
  (es lo normal).
- **Aparece un archivo pero Golden no lo importa** → comprueba que la ruta de
  importación en Golden coincide exactamente con la carpeta configurada.

## Actualizar

Ejecuta el nuevo `invoicesup-agent-setup.exe`: detiene la versión anterior, la
reemplaza y vuelve a iniciar el servicio, conservando tu configuración.

## Desinstalar

Panel de control → *Programas* → *InvoicesUp Connector Agent* → *Desinstalar*.
Detiene y quita el servicio y borra la configuración (incluido el token).
