DELETE FROM `groups` WHERE `kind` = 'direct';
DROP INDEX `groups_direct_key_idx`;
ALTER TABLE `groups` DROP COLUMN `direct_key`;
ALTER TABLE `groups` DROP COLUMN `kind`;
