CREATE TABLE account (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  region INT NOT NULL,
  parent_id BIGINT UNSIGNED NULL DEFAULT NULL,
  parent_region INT NULL,
  code VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' COMMENT 'Public code',
  amount DECIMAL(12,3) NOT NULL DEFAULT -1.250,
  label VARCHAR(30) NOT NULL DEFAULT 'NULL',
  status ENUM('new','it''s ready') NOT NULL DEFAULT 'new',
  tags SET('one','two') NULL DEFAULT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  doubled DECIMAL(14,3) GENERATED ALWAYS AS (amount * 2) STORED,
  PRIMARY KEY (id, region),
  INDEX parent_account (parent_id, parent_region),
  INDEX code_prefix (code(8) DESC) INVISIBLE COMMENT 'Lookup prefix',
  CONSTRAINT account_parent FOREIGN KEY (parent_id, parent_region)
    REFERENCES account (id, region) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin COMMENT='Accounts';

CREATE TABLE entry (
  id INT NOT NULL,
  account_id BIGINT UNSIGNED NOT NULL,
  region INT NOT NULL,
  PRIMARY KEY (id),
  INDEX entry_account (account_id, region),
  INDEX entry_concept (region),
  CONSTRAINT entry_account_fk FOREIGN KEY (account_id, region)
    REFERENCES account (id, region) ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=ascii COLLATE=ascii_bin;
