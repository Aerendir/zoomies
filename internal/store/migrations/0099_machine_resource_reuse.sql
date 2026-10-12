-- A hypervisor reuses its ids. Proxmox is asked for the lowest free VMID in the
-- range an operator allowed, so once a machine's VM has been destroyed and the
-- provider has confirmed it gone, the next machine is given the same id.
--
-- The unique index on a machine's resource counted every row that ever named
-- one, history included. The first deleted machine therefore kept its VMID for
-- ever, every later allocation picked that id again, and the write that records
-- it before anything is built failed with a constraint error: the provider could
-- build no second machine, and the failures stood it down.
--
-- deleted_at is when the provider confirmed the resource gone, and is already how
-- idx_machines_owned decides that a row still owns something. The unique index
-- now agrees with it. While a resource exists at most one row may name it; once
-- it is confirmed gone the row stays, as the record of what was rented and what
-- it cost, and stops claiming the id.
DROP INDEX idx_machines_resource;
CREATE UNIQUE INDEX idx_machines_resource ON machines(provider_id, resource_zone, resource_id)
    WHERE resource_id <> '' AND deleted_at IS NULL;
