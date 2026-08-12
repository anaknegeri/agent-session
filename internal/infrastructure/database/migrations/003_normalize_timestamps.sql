-- Timestamps are stored as TEXT and every ORDER BY compares them as strings, so
-- string order has to match chronological order. The previous encoding
-- (RFC3339Nano) trimmed trailing zeros from the fraction, giving variable-width
-- values: a whole second was written "10:00:00Z" and sorted after the later
-- "10:00:00.5Z", because 'Z' is greater than '.'. Writes now use a fixed
-- nine-digit fraction; existing rows are padded here so old and new values
-- compare correctly against each other.
--
-- Values are UTC and end in 'Z'. A value with a fraction keeps its digits padded
-- to nine; a value without one gains '.000000000'.

UPDATE projects SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE sessions SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE sessions SET updated_at =
    CASE WHEN instr(updated_at, '.') > 0
        THEN substr(updated_at, 1, instr(updated_at, '.'))
             || substr(substr(updated_at, instr(updated_at, '.') + 1, length(updated_at) - instr(updated_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(updated_at, 1, length(updated_at) - 1) || '.000000000Z'
    END
WHERE updated_at LIKE '%Z' AND length(updated_at) <> 30;

UPDATE tasks SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE tasks SET updated_at =
    CASE WHEN instr(updated_at, '.') > 0
        THEN substr(updated_at, 1, instr(updated_at, '.'))
             || substr(substr(updated_at, instr(updated_at, '.') + 1, length(updated_at) - instr(updated_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(updated_at, 1, length(updated_at) - 1) || '.000000000Z'
    END
WHERE updated_at LIKE '%Z' AND length(updated_at) <> 30;

UPDATE decisions SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE blockers SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE blockers SET resolved_at =
    CASE WHEN instr(resolved_at, '.') > 0
        THEN substr(resolved_at, 1, instr(resolved_at, '.'))
             || substr(substr(resolved_at, instr(resolved_at, '.') + 1, length(resolved_at) - instr(resolved_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(resolved_at, 1, length(resolved_at) - 1) || '.000000000Z'
    END
WHERE resolved_at LIKE '%Z' AND length(resolved_at) <> 30;

UPDATE session_events SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE checkpoints SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE agent_sessions SET started_at =
    CASE WHEN instr(started_at, '.') > 0
        THEN substr(started_at, 1, instr(started_at, '.'))
             || substr(substr(started_at, instr(started_at, '.') + 1, length(started_at) - instr(started_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(started_at, 1, length(started_at) - 1) || '.000000000Z'
    END
WHERE started_at LIKE '%Z' AND length(started_at) <> 30;

UPDATE agent_sessions SET ended_at =
    CASE WHEN instr(ended_at, '.') > 0
        THEN substr(ended_at, 1, instr(ended_at, '.'))
             || substr(substr(ended_at, instr(ended_at, '.') + 1, length(ended_at) - instr(ended_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(ended_at, 1, length(ended_at) - 1) || '.000000000Z'
    END
WHERE ended_at LIKE '%Z' AND length(ended_at) <> 30;

UPDATE artifacts SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;

UPDATE knowledge SET created_at =
    CASE WHEN instr(created_at, '.') > 0
        THEN substr(created_at, 1, instr(created_at, '.'))
             || substr(substr(created_at, instr(created_at, '.') + 1, length(created_at) - instr(created_at, '.') - 1) || '000000000', 1, 9)
             || 'Z'
        ELSE substr(created_at, 1, length(created_at) - 1) || '.000000000Z'
    END
WHERE created_at LIKE '%Z' AND length(created_at) <> 30;
