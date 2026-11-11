ALTER TABLE public.channels
  DROP COLUMN IF EXISTS emoji_status_collectible,
  DROP COLUMN IF EXISTS emoji_status_collectible_id;
