-- Fix api_key_prefix column length (prefix is 12 chars: "ahk_" + 8 hex)
ALTER TABLE connections ALTER COLUMN api_key_prefix TYPE VARCHAR(16);

-- Fix token_prefix column length (prefix is 12 chars: "aht_" + 8 hex)
ALTER TABLE scoped_tokens ALTER COLUMN token_prefix TYPE VARCHAR(16);

-- Fix memory_scratch.date column type for comparison with text
-- The GenerateDailyScratchPlaceholders uses date = text comparison
ALTER TABLE memory_scratch ALTER COLUMN date TYPE DATE USING date::DATE;
