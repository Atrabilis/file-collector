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
- `growing_file`: procesa solo el archivo mas reciente, persiste estado de lectura y retoma desde el ultimo offset confirmado

Para archivos historicos acumulativos tipo `CR1000X`, se puede usar:

```yaml
inputs:
  - name: meteo_psda
    directory: /ruta/a/Radiometria
    mode: growing_file
    replay_lines: 5
    state_directory: /var/lib/atamostec/file-collector/state
    timestamp_column: TIMESTAMP
    include:
      - "CRx_22568_Meteo.dat"
```

En `growing_file` el collector:

- guarda estado por `input + tabla destino`
- retoma desde el ultimo `offset` confirmado despues de `commit`
- reprocesa por defecto las ultimas `5` filas para tolerar cortes breves
- si el archivo no cambio, no lo relee
- si el archivo se achica o cambia de nombre, reinicia desde el comienzo

`storage.outputs` soporta por ahora:

- `timescaledb`: destino PostgreSQL/TimescaleDB con valores directos o vía variables de entorno

Adicionalmente, cada `input` puede exportar el ultimo valor procesado como metricas Prometheus usando el `textfile collector` de `node_exporter`:

```yaml
inputs:
  - name: psda__meteo_6857
    directory: /mnt/ALMACENAMIENTO/ATAMOSTEC/Data/Radiometria
    mode: growing_file
    timestamp_column: ts
    timestamp_source_columns:
      - TIMESTAMP
    timestamp_layouts:
      - "2006-01-02 15:04:05"
    include:
      - "CRx_6857_Meteo.dat"
    prometheus_textfile:
      enabled: true
      directory: /var/lib/node_exporter/textfile_collector
      file_name: psda__meteo_6857.prom
      metrics:
        - name: atamostec_radiometry_dni_avg_watts_per_square_meter
          help: Latest DNI average from file-collector.
          value_column: DNI_Avg
          labels:
            plant: psda
            filename: CRx_6857_Meteo.dat
          timestamp_metric_name: atamostec_radiometry_sample_timestamp_seconds
          timestamp_metric_help: Timestamp of the latest source sample.
```

En ese modo el collector:

- toma la ultima fila procesada exitosamente del `input`
- reconstruye el archivo `.prom` completo en cada refresh
- reemplaza el archivo de forma atomica, sin `append`
- deja desaparecer metricas obsoletas simplemente dejando de escribirlas

Opciones relevantes de `timescaledb`:

- `on_conflict`: `do_update` o `do_nothing`
- `batch_size`: techo de filas por `INSERT ... VALUES ... ON CONFLICT ...`. El collector lo reduce automaticamente si hace falta para no superar el limite de parametros de PostgreSQL. Default: `5000`

Comportamientos por defecto del writer:

- columnas presentes en el archivo pero ausentes en la tabla se omiten y se informan en logs
- columnas presentes en la tabla pero ausentes en el archivo quedan en `NULL` si la tabla no define otro default
- valores `NaN` se convierten a `NULL`
