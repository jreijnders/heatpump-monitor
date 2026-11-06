-- Energy prices from the easyEnergy dynamic contract (ADR-0004) and the Break-even COP against a
-- gas boiler. Written by the prices service (ingester image, cmd/prices). Idempotent.

-- energy_prices: one row per price interval as published by easyEnergy (price-graph API).
--   commodity      'electricity' (quarter-hour intervals, EUR/kWh) or 'gas' (local day, EUR/m3).
--   starts, ends   interval [starts, ends); easyEnergy publishes Europe/Amsterdam wall-clock times.
--   market_price_ex_vat / market_price_inc_vat   day-ahead market price.
--   energy_tax     energiebelasting incl. VAT.
--   purchase_fee   easyEnergy's inkoopvergoeding incl. VAT (their per-unit fee).
--   invoice_price  all-in per-unit price = market_price_inc_vat + energy_tax + purchase_fee.
-- Fixed charges (vaste leveringskosten, netbeheer, vermindering energiebelasting) are per
-- connection, not per unit, so they do not affect the Break-even COP and are not stored.
CREATE TABLE IF NOT EXISTS energy_prices (
    commodity            text        NOT NULL CHECK (commodity IN ('electricity', 'gas')),
    starts               timestamptz NOT NULL,
    ends                 timestamptz NOT NULL CHECK (ends > starts),
    unit                 text        NOT NULL,
    market_price_ex_vat  float8      NOT NULL,
    market_price_inc_vat float8      NOT NULL,
    energy_tax           float8      NOT NULL,
    purchase_fee         float8      NOT NULL,
    invoice_price        float8      NOT NULL,
    fetched_at           timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (commodity, starts)
);

GRANT SELECT, INSERT, UPDATE ON energy_prices TO ingester;
GRANT SELECT ON energy_prices TO analytics_ro;

-- gas_kwh_per_m3(): gross calorific value of Dutch billing gas (35.17 MJ/m3 = 9.769 kWh/m3).
CREATE OR REPLACE FUNCTION gas_kwh_per_m3() RETURNS float8
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT 9.769::float8 $$;

-- gas_boiler_efficiency(): heat delivered per unit of gas energy (gross calorific value) of the
-- gas boiler the Heat Pump is compared with. 0.963 = HR107 label value (107 % on the lower
-- calorific value / 1.111 upper-to-lower ratio). Higher efficiency -> cheaper gas heat -> higher
-- Break-even COP. TUNE HERE: ~0.90 for a condensing boiler on radiators (return >= 40 C),
-- ~0.85 for an older or oversized boiler.
CREATE OR REPLACE FUNCTION gas_boiler_efficiency() RETURNS float8
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT 0.963::float8 $$;

-- breakeven_cop: per electricity price interval, the Break-even COP: the COP at which heat from
-- the Heat Pump costs the same as heat from the gas boiler.
--   gas_heat_price_eur_kwh = gas invoice price / (gas_kwh_per_m3() * gas_boiler_efficiency())
--   breakeven_cop          = elec_price_eur_kwh / gas_heat_price_eur_kwh
-- Gas price of the interval's local day. Independent of whether the Heat Pump ran.
CREATE OR REPLACE VIEW breakeven_cop AS
SELECT
    e.starts,
    e.ends,
    e.invoice_price                                                        AS elec_price_eur_kwh,
    g.invoice_price                                                        AS gas_price_eur_m3,
    g.invoice_price / (gas_kwh_per_m3() * gas_boiler_efficiency())        AS gas_heat_price_eur_kwh,
    e.invoice_price / NULLIF(g.invoice_price / (gas_kwh_per_m3() * gas_boiler_efficiency()), 0)
                                                                           AS breakeven_cop
FROM energy_prices e
JOIN energy_prices g
  ON g.commodity = 'gas'
 AND (g.starts AT TIME ZONE local_tz())::date = (e.starts AT TIME ZONE local_tz())::date
WHERE e.commodity = 'electricity';

-- energy_cost_1m: per minute of energy_1m with both heat and electricity known (same minutes as
-- the COP views): what the electricity cost, and what the same heat would have cost from gas.
--   elec_cost_eur = elec_kwh * elec_price_eur_kwh (quarter-hour price containing the minute)
--   gas_cost_eur  = heat_kwh * gas_heat_price_eur_kwh
-- Minutes without a price (before the first fetched price, or the repeated 02:00 hour on the
-- autumn DST day, which easyEnergy does not publish separately) are absent.
CREATE OR REPLACE VIEW energy_cost_1m AS
SELECT
    m.bucket,
    m.operating_mode,
    m.heat_kwh,
    m.elec_kwh,
    b.elec_price_eur_kwh,
    b.gas_heat_price_eur_kwh,
    b.breakeven_cop,
    m.elec_kwh * b.elec_price_eur_kwh      AS elec_cost_eur,
    m.heat_kwh * b.gas_heat_price_eur_kwh  AS gas_cost_eur
FROM energy_1m m
JOIN breakeven_cop b ON b.starts = time_bucket('15 minutes', m.bucket)
WHERE m.heat_kwh IS NOT NULL AND m.elec_kwh IS NOT NULL;

-- energy_cost_daily: per local day and operating_mode, plus 'Combined' (as cop_daily).
--   cop             Σheat / Σelec over minutes that have prices.
--   breakeven_cop   consumption-weighted: (Σelec_cost / Σelec) / (Σgas_cost / Σheat), i.e. the
--                   average electricity price actually paid divided by the gas heat price.
--                   cop > breakeven_cop  <=>  elec_cost_eur < gas_cost_eur.
--   savings_eur     gas_cost_eur - elec_cost_eur (negative: gas would have been cheaper).
CREATE OR REPLACE VIEW energy_cost_daily AS
SELECT
    time_bucket('1 day', bucket, local_tz())                  AS bucket,
    coalesce(operating_mode, 'Combined')::text                AS operating_mode,
    sum(heat_kwh)                                             AS heat_kwh,
    sum(elec_kwh)                                             AS elec_kwh,
    sum(heat_kwh) / NULLIF(sum(elec_kwh), 0)                  AS cop,
    (sum(elec_cost_eur) / NULLIF(sum(elec_kwh), 0))
      / NULLIF(sum(gas_cost_eur) / NULLIF(sum(heat_kwh), 0), 0) AS breakeven_cop,
    sum(elec_cost_eur)                                        AS elec_cost_eur,
    sum(gas_cost_eur)                                         AS gas_cost_eur,
    sum(gas_cost_eur) - sum(elec_cost_eur)                    AS savings_eur,
    count(*)::int                                             AS minutes
FROM energy_cost_1m
GROUP BY GROUPING SETS ((time_bucket('1 day', bucket, local_tz()), operating_mode),
                        (time_bucket('1 day', bucket, local_tz())));

GRANT SELECT ON breakeven_cop, energy_cost_1m, energy_cost_daily TO analytics_ro;
