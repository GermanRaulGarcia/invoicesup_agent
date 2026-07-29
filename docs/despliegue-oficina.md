# Guía de despliegue — Oficina contable

Guía corta para instalar el **Agente InvoicesUp** en la PC de la oficina. El
agente trae automáticamente las facturas desde InvoicesUp y las deja como
archivos que **Golden** importa.

## Qué necesitás antes de empezar

- Windows 10/11 (o Windows Server) con permisos de **administrador**.
- Estos tres datos, que te pasa el administrador de InvoicesUp:
  1. **URL** de InvoicesUp (ej. `https://invoicesup.kordino.com`).
  2. **Token de conector** (una clave larga; se genera en InvoicesUp → Usuarios
     → editar el usuario de la oficina → *Generar token*).
  3. **Carpeta local** donde querés que aparezcan los archivos (ej.
     `C:\InvoicesUp\exports`). Es una carpeta del disco, no una unidad de red.

## Instalación

1. Ejecutá **`invoicesup-agent-setup.exe`** (clic derecho → *Ejecutar como
   administrador*).
2. En la pantalla *Configuración del agente*, completá los tres datos de arriba.
3. Terminá el asistente. Al finalizar, el agente queda **instalado como servicio
   de Windows** y arranca solo — no hay que abrir ninguna ventana.

A partir de ahí arranca con la máquina y corre en segundo plano.

## Verificar que anda

- Abrí **Servicios** (`services.msc`) y buscá **“InvoicesUp Connector Agent”**:
  debe estar en estado **En ejecución**.
- Cuando el administrador exporte facturas desde InvoicesUp, en unos ~30 segundos
  van apareciendo archivos `CODIGO_facturas.txt` en tu carpeta (uno por empresa).

## Configurar Golden

- En Golden, apuntá la importación a tu carpeta local (ej.
  `C:\InvoicesUp\exports\SPM_facturas.txt`) — **un archivo por empresa**, según el
  código de cada una.
- Activá en Golden la opción de **eliminar el archivo tras importar**. El agente
  detecta ese borrado, avisa a InvoicesUp que ya lo importaste, y trae las
  próximas facturas nuevas. Así el ciclo se repite solo.

## Si algo no funciona

- **El servicio no arranca / se detiene solo** → revisá el registro en
  `C:\ProgramData\InvoicesUp\agent.log`. Suele ser un dato mal cargado (URL,
  token o carpeta). Reinstalá corrigiendo el dato.
- **No aparecen archivos** → confirmá con el administrador que (a) el token es el
  correcto y sigue activo, y (b) que la oficina tiene empresas asignadas y hay
  facturas exportadas. Sin facturas nuevas, no se escribe ningún archivo (es
  normal).
- **Aparece un archivo pero Golden no lo importa** → verificá que la ruta de
  importación en Golden coincide exactamente con la carpeta configurada.

## Actualizar

Ejecutá el nuevo `invoicesup-agent-setup.exe`: detiene la versión anterior,
la reemplaza y vuelve a arrancar el servicio, conservando tu configuración.

## Desinstalar

Panel de control → *Programas* → *InvoicesUp Connector Agent* → *Desinstalar*.
Detiene y quita el servicio y borra la configuración (incluido el token).
