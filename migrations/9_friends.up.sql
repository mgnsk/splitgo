-- Friends share a hidden two-person "direct" group holding their non-group
-- expenses. direct_key is "<smaller user id>:<larger user id>" so a pair has
-- at most one.
ALTER TABLE `groups` ADD COLUMN `kind` text NOT NULL DEFAULT 'group' CHECK (`kind` IN ('group', 'direct'));
ALTER TABLE `groups` ADD COLUMN `direct_key` text;

CREATE UNIQUE INDEX `groups_direct_key_idx` ON `groups`(`direct_key`) WHERE `direct_key` IS NOT NULL;
