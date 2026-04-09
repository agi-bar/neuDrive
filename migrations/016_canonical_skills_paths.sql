-- Canonicalize legacy .skills/* entries into /skills/*.

-- If a canonical /skills/* row already exists for the same user and logical path,
-- drop the legacy duplicate before rewriting remaining rows.
DELETE FROM file_tree AS legacy
USING file_tree AS canonical
WHERE legacy.user_id = canonical.user_id
  AND legacy.id <> canonical.id
  AND (legacy.path = '.skills' OR legacy.path LIKE '.skills/%')
  AND canonical.path = regexp_replace(legacy.path, '^\.skills', '/skills');

UPDATE file_tree
SET path = regexp_replace(path, '^\.skills', '/skills'),
    checksum = encode(digest(
        regexp_replace(path, '^\.skills', '/skills') || '|' ||
        coalesce(content, '') || '|' ||
        coalesce(content_type, '') || '|' ||
        coalesce(metadata::text, '{}'),
        'sha256'
    ), 'hex')
WHERE path LIKE '.skills/%';
-- Also handle a rare bare ".skills" root entry if one exists.
UPDATE file_tree
SET path = '/skills',
    checksum = encode(digest(
        '/skills|' ||
        coalesce(content, '') || '|' ||
        coalesce(content_type, '') || '|' ||
        coalesce(metadata::text, '{}'),
        'sha256'
    ), 'hex')
WHERE path = '.skills';

UPDATE entry_versions
SET path = regexp_replace(path, '^\.skills', '/skills'),
    checksum = encode(digest(
        regexp_replace(path, '^\.skills', '/skills') || '|' ||
        coalesce(content, '') || '|' ||
        coalesce(content_type, '') || '|' ||
        coalesce(metadata::text, '{}'),
        'sha256'
    ), 'hex')
WHERE path LIKE '.skills/%';

UPDATE entry_versions
SET path = '/skills',
    checksum = encode(digest(
        '/skills|' ||
        coalesce(content, '') || '|' ||
        coalesce(content_type, '') || '|' ||
        coalesce(metadata::text, '{}'),
        'sha256'
    ), 'hex')
WHERE path = '.skills';
