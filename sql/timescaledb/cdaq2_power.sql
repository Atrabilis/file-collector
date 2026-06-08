BEGIN;

CREATE SCHEMA IF NOT EXISTS ua;

CREATE TABLE IF NOT EXISTS ua.cdaq2_ac_power (
    ts TIMESTAMPTZ NOT NULL,
    source_file TEXT NOT NULL,
    source_line_number INTEGER NOT NULL,
    flags JSONB NOT NULL DEFAULT '{}'::jsonb,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    frequency_invert_1_hz DOUBLE PRECISION,
    voltagef_1_invert_1_v DOUBLE PRECISION,
    voltagef_2_invert_1_v DOUBLE PRECISION,
    voltagef_3_invert_1_v DOUBLE PRECISION,
    voltagef_n_invert_1_v DOUBLE PRECISION,
    voltage_u_12_invert_1_v DOUBLE PRECISION,
    voltage_u_23_invert_1_v DOUBLE PRECISION,
    voltage_u_31_invert_1_v DOUBLE PRECISION,
    current_1_invert_1_a DOUBLE PRECISION,
    current_2_invert_1_a DOUBLE PRECISION,
    current_3_invert_1_a DOUBLE PRECISION,
    current_in_inversor_1_a DOUBLE PRECISION,
    total_active_power_invert_1_w DOUBLE PRECISION,
    total_reactive_power_invert_1_var DOUBLE PRECISION,
    total_apparent_power_invert_1_w DOUBLE PRECISION,
    total_power_factor DOUBLE PRECISION,
    active_power_1_invert_1_w DOUBLE PRECISION,
    active_power_2_invert_1_w DOUBLE PRECISION,
    active_power_3_invert_1_w DOUBLE PRECISION,
    reactive_power_1_invert_1_var DOUBLE PRECISION,
    reactive_power_2_invert_1_var DOUBLE PRECISION,
    reactive_power_3_invert_1_var DOUBLE PRECISION,
    apparent_power_s_1_invert_1_w DOUBLE PRECISION,
    apparent_power_s_2_invert_1_w DOUBLE PRECISION,
    apparent_power_s_3_invert_1_w DOUBLE PRECISION,
    power_factor_1 DOUBLE PRECISION,
    power_factor_2 DOUBLE PRECISION,
    power_factor_3 DOUBLE PRECISION,
    thd_v_1_pct DOUBLE PRECISION,
    thd_v_2_pct DOUBLE PRECISION,
    thd_v_3_pct DOUBLE PRECISION,
    thd_u_1_pct DOUBLE PRECISION,
    thd_u_2_pct DOUBLE PRECISION,
    thd_u_3_pct DOUBLE PRECISION,
    thd_i_1_pct DOUBLE PRECISION,
    thd_i_2_pct DOUBLE PRECISION,
    thd_i_3_pct DOUBLE PRECISION,
    thd_in_pct DOUBLE PRECISION,
    thdv_syst_pct DOUBLE PRECISION,
    thdu_syst_pct DOUBLE PRECISION,
    thdi_syst_pct DOUBLE PRECISION,
    CONSTRAINT cdaq2_ac_power_pkey PRIMARY KEY (ts)
);

SELECT create_hypertable(
    'ua.cdaq2_ac_power',
    'ts',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE,
    migrate_data => TRUE
);

CREATE INDEX IF NOT EXISTS cdaq2_ac_power_ts_idx
    ON ua.cdaq2_ac_power (ts DESC);

CREATE INDEX IF NOT EXISTS cdaq2_ac_power_ingested_at_idx
    ON ua.cdaq2_ac_power (ingested_at DESC);

CREATE INDEX IF NOT EXISTS cdaq2_ac_power_source_file_idx
    ON ua.cdaq2_ac_power (source_file, ts DESC);

COMMENT ON TABLE ua.cdaq2_ac_power IS
    'Structured AC power records ingested from CDAQ2 delimited files.';

COMMENT ON COLUMN ua.cdaq2_ac_power.flags IS
    'Collector metadata for the row, e.g. parse warnings or provenance details.';

CREATE TABLE IF NOT EXISTS ua.cdaq2_dc_power (
    ts TIMESTAMPTZ NOT NULL,
    source_file TEXT NOT NULL,
    source_line_number INTEGER NOT NULL,
    flags JSONB NOT NULL DEFAULT '{}'::jsonb,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    "2_ve_201_v" DOUBLE PRECISION,
    "2_ie_201_a" DOUBLE PRECISION,
    "2_pe_201_w" DOUBLE PRECISION,
    "2_ve_202_v" DOUBLE PRECISION,
    "2_ie_202_a" DOUBLE PRECISION,
    "2_pe_202_w" DOUBLE PRECISION,
    "2_ve_203_v" DOUBLE PRECISION,
    "2_ie_203_a" DOUBLE PRECISION,
    "2_pe_203_w" DOUBLE PRECISION,
    "2_ve_401_v" DOUBLE PRECISION,
    "2_ie_401_a" DOUBLE PRECISION,
    "2_pe_401_w" DOUBLE PRECISION,
    "2_ve_402_v" DOUBLE PRECISION,
    "2_ie_402_a" DOUBLE PRECISION,
    "2_pe_402_w" DOUBLE PRECISION,
    "2_ve_403_v" DOUBLE PRECISION,
    "2_ie_403_a" DOUBLE PRECISION,
    "2_pe_403_w" DOUBLE PRECISION,
    "2_ve_404_v" DOUBLE PRECISION,
    "2_ie_404_a" DOUBLE PRECISION,
    "2_pe_404_w" DOUBLE PRECISION,
    "2_ve_405_v" DOUBLE PRECISION,
    "2_ie_405_a" DOUBLE PRECISION,
    "2_pe_405_w" DOUBLE PRECISION,
    "2_ve_406_v" DOUBLE PRECISION,
    "2_ie_406_a" DOUBLE PRECISION,
    "2_pe_406_w" DOUBLE PRECISION,
    "2_ve_601_v" DOUBLE PRECISION,
    "2_ie_601_a" DOUBLE PRECISION,
    "2_pe_601_w" DOUBLE PRECISION,
    "2_ve_602_v" DOUBLE PRECISION,
    "2_ie_602_a" DOUBLE PRECISION,
    "2_pe_602_w" DOUBLE PRECISION,
    "2_ve_603_v" DOUBLE PRECISION,
    "2_ie_603_a" DOUBLE PRECISION,
    "2_pe_603_w" DOUBLE PRECISION,
    "2_ve_604_v" DOUBLE PRECISION,
    "2_ie_604_a" DOUBLE PRECISION,
    "2_pe_604_w" DOUBLE PRECISION,
    "2_ve_605_v" DOUBLE PRECISION,
    "2_ie_605_a" DOUBLE PRECISION,
    "2_pe_605_w" DOUBLE PRECISION,
    "2_ve_606_v" DOUBLE PRECISION,
    "2_ie_606_a" DOUBLE PRECISION,
    "2_pe_606_w" DOUBLE PRECISION,
    "2_ve_701_v" DOUBLE PRECISION,
    "2_ie_701_a" DOUBLE PRECISION,
    "2_pe_701_w" DOUBLE PRECISION,
    "2_ve_702_v" DOUBLE PRECISION,
    "2_ie_702_a" DOUBLE PRECISION,
    "2_pe_702_w" DOUBLE PRECISION,
    "2_ve_703_v" DOUBLE PRECISION,
    "2_ie_703_a" DOUBLE PRECISION,
    "2_pe_703_w" DOUBLE PRECISION,
    "2_ve_704_v" DOUBLE PRECISION,
    "2_ie_704_a" DOUBLE PRECISION,
    "2_pe_704_w" DOUBLE PRECISION,
    "2_ve_705_v" DOUBLE PRECISION,
    "2_ie_705_a" DOUBLE PRECISION,
    "2_pe_705_w" DOUBLE PRECISION,
    "2_ve_706_v" DOUBLE PRECISION,
    "2_ie_706_a" DOUBLE PRECISION,
    "2_pe_706_w" DOUBLE PRECISION,
    CONSTRAINT cdaq2_dc_power_pkey PRIMARY KEY (ts)
);

SELECT create_hypertable(
    'ua.cdaq2_dc_power',
    'ts',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE,
    migrate_data => TRUE
);

CREATE INDEX IF NOT EXISTS cdaq2_dc_power_ts_idx
    ON ua.cdaq2_dc_power (ts DESC);

CREATE INDEX IF NOT EXISTS cdaq2_dc_power_ingested_at_idx
    ON ua.cdaq2_dc_power (ingested_at DESC);

CREATE INDEX IF NOT EXISTS cdaq2_dc_power_source_file_idx
    ON ua.cdaq2_dc_power (source_file, ts DESC);

COMMENT ON TABLE ua.cdaq2_dc_power IS
    'Structured DC power records ingested from CDAQ2 delimited files.';

COMMENT ON COLUMN ua.cdaq2_dc_power.flags IS
    'Collector metadata for the row, e.g. parse warnings or provenance details.';

COMMIT;
