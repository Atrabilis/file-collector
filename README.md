# file-collector

Primer esqueleto para leer archivos tabulares delimitados desde disco.

Por ahora hace solo esto:

- carga configuracion YAML con `inputs`
- recorre un archivo o directorio
- detecta delimitador comun (`;`, `,`, tab, `|`)
- lee header y filas
- soporta tipos explicitos por columna con fallback por inferencia
- reporta metadatos basicos por archivo
- reporta filas mal formadas y errores de parseo para columnas tipadas
- soporta destinos TimescaleDB declarados en YAML por input

Ejemplo:

```bash
go run ./cmd/file-collector read --path /ruta/a/Power --glob '*power.txt'
```

Ejemplo de configuracion:

```yaml
inputs:
  - name: ac_power
    directory: /media/operadorua/ALMACENAMIENTO/ATAMOSTEC2/Data/CDAQ2/Power
    mode: latest
    timestamp_column: TimeStamp
    delimiter: ";"
    decimal_comma: true
    include:
      - "*_ACpower.txt"
    storage:
      outputs:
        - name: local_timescale
          type: timescaledb
          enabled: true
          timescaledb:
            host_env: TIMESCALE_HOST_LOCAL
            port_env: TIMESCALE_PORT_LOCAL
            user_env: TIMESCALE_USER_LOCAL
            password_env: TIMESCALE_PASSWORD_LOCAL
            database_env: TIMESCALE_DB_LOCAL
            schema: ua
            table: cdaq2_ac_power
            sslmode: disable
            batch_size: 5000

  - name: dc_power
    directory: /media/operadorua/ALMACENAMIENTO/ATAMOSTEC2/Data/CDAQ2/Power
    mode: latest
    timestamp_column: TimeStamp
    delimiter: ";"
    decimal_comma: true
    include:
      - "*_DCpower.txt"
    storage:
      outputs:
        - name: local_timescale
          type: timescaledb
          enabled: true
          timescaledb:
            host_env: TIMESCALE_HOST_LOCAL
            port_env: TIMESCALE_PORT_LOCAL
            user_env: TIMESCALE_USER_LOCAL
            password_env: TIMESCALE_PASSWORD_LOCAL
            database_env: TIMESCALE_DB_LOCAL
            schema: ua
            table: cdaq2_dc_power
            sslmode: disable
            batch_size: 5000
```

Para archivos que traen fecha y hora separadas, se puede construir el timestamp con:

```yaml
timestamp_column: ts
timestamp_source_columns:
  - Date
  - time
timestamp_layouts:
  - "02.01.2006 15:04:05"
```

En ese caso el collector:

- concatena las columnas fuente con espacio
- parsea el timestamp usando `timestamp_layouts`
- inserta `ts` como marca de tiempo
- no persiste las columnas fuente del timestamp

`mode` soporta por ahora:

- `all`: procesa todos los archivos matcheados
- `latest`: procesa solo el archivo con `mtime` más reciente
- `last_n_files`: procesa los `N` archivos con `mtime` más reciente usando `last_n_files`

`storage.outputs` soporta por ahora:

- `timescaledb`: destino PostgreSQL/TimescaleDB con valores directos o vía variables de entorno

Opciones relevantes de `timescaledb`:

- `on_conflict`: `do_update` o `do_nothing`
- `batch_size`: techo de filas por `INSERT ... VALUES ... ON CONFLICT ...`. El collector lo reduce automaticamente si hace falta para no superar el limite de parametros de PostgreSQL. Default: `5000`
