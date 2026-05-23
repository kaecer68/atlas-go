-- Revert alerts.id back to UUID.
-- Only safe if all existing IDs are valid UUIDs.

ALTER TABLE alerts DROP CONSTRAINT alerts_pkey;
ALTER TABLE alerts ALTER COLUMN id TYPE UUID USING id::UUID;
ALTER TABLE alerts ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE alerts ADD PRIMARY KEY (id);
