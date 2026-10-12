-- The certificate a machine trusts for the controller, apart from the one the
-- controller trusts for the provider.
--
-- providers.ca_pem did both jobs. Every machine was handed it as the only
-- authority it would accept from this controller, so a cluster's own authority
-- became the one thing a machine could verify the controller against, and a
-- controller on a public certificate was refused by every machine it rented.
-- The column below holds the machines' half on its own. Empty means a machine
-- uses its own trust store.
--
-- A provider that already has a certificate is given it here as well. That is
-- what its machines were being told before this migration, and a deployment
-- whose controller is signed by the same private authority as its hypervisor
-- depends on it. Nothing changes for such a provider until somebody edits the
-- field, which the form now explains.
ALTER TABLE providers ADD COLUMN machine_ca_pem TEXT NOT NULL DEFAULT '';
UPDATE providers SET machine_ca_pem = ca_pem WHERE ca_pem <> '';
