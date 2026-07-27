-- targets: a system Vantage is authorized to measure (SPEC §6).
-- Config, not evidence — targets may be edited (re-allowlisted, retargeted).
CREATE TABLE targets (
    id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name             text        NOT NULL UNIQUE,
    base_url         text        NOT NULL,
    request_template jsonb       NOT NULL DEFAULT '{}'::jsonb,
    auth             jsonb,
    mode             text        NOT NULL,
    mutating         boolean     NOT NULL DEFAULT false,
    allowlisted      boolean     NOT NULL DEFAULT false,
    created_at       timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT targets_mode_valid CHECK (mode IN ('black-box', 'white-box')),

    -- Secrets gate (SPEC review #5, CLAUDE §7): auth is a reference or an
    -- encrypted envelope (e.g. {"scheme":"secretbox","enc":"…","nonce":"…"} or
    -- {"secret_ref":"…"}) — NEVER a plaintext bearer token. Forbid the obvious
    -- plaintext keys in the schema so the invariant lives in code, not a doc.
    -- ('secret_ref' is allowed; only the bare 'secret' key is rejected.)
    CONSTRAINT targets_auth_no_plaintext CHECK (
        auth IS NULL
        OR NOT (auth ? 'token' OR auth ? 'password' OR auth ? 'bearer' OR auth ? 'secret')
    )
);
