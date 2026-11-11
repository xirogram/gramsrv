ALTER TABLE public.channels
  ADD COLUMN IF NOT EXISTS emoji_status_collectible_id bigint REFERENCES public.unique_star_gifts(id),
  ADD COLUMN IF NOT EXISTS emoji_status_collectible jsonb DEFAULT '{}'::jsonb NOT NULL;
