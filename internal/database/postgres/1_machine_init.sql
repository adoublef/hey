create schema machine;

create table machine.machine (
    id uuid primary key
    , metadata jsonb -- { ..., state }
    , modified_at timestamptz
);

create index idx_machine_id_modified_at on machine.machine (id, modified_at);
create index idx_machine_metadata_gin on machine.machine using gin ((metadata -> 'state'));

---- create above / drop below ----

drop schema machine cascade;
