-- Project Management: an independently authored MySQL 8 schema.
-- This SQL and the editable example describe the same schema. They are maintained
-- separately to exercise import and generation against a meaningful reference.

CREATE TABLE member (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  email VARCHAR(254) NOT NULL,
  display_name VARCHAR(120) NOT NULL,
  timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY member_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE workspace (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  slug VARCHAR(64) NOT NULL,
  name VARCHAR(160) NOT NULL,
  settings JSON NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY workspace_slug (slug)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE workspace_member (
  workspace_id BIGINT UNSIGNED NOT NULL,
  member_id BIGINT UNSIGNED NOT NULL,
  role ENUM('owner', 'admin', 'member') NOT NULL DEFAULT 'member',
  joined_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (workspace_id, member_id),
  KEY workspace_member_member (member_id),
  CONSTRAINT workspace_member_workspace_fk FOREIGN KEY (workspace_id) REFERENCES workspace (id) ON DELETE CASCADE,
  CONSTRAINT workspace_member_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE team (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  workspace_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(120) NOT NULL,
  description TEXT NULL,
  PRIMARY KEY (id),
  UNIQUE KEY team_workspace_name (workspace_id, name),
  CONSTRAINT team_workspace_fk FOREIGN KEY (workspace_id) REFERENCES workspace (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE team_member (
  team_id BIGINT UNSIGNED NOT NULL,
  member_id BIGINT UNSIGNED NOT NULL,
  joined_on DATE NOT NULL,
  PRIMARY KEY (team_id, member_id),
  KEY team_member_member (member_id),
  CONSTRAINT team_member_team_fk FOREIGN KEY (team_id) REFERENCES team (id) ON DELETE CASCADE,
  CONSTRAINT team_member_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE project (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  workspace_id BIGINT UNSIGNED NOT NULL,
  code CHAR(12) NOT NULL,
  name VARCHAR(180) NOT NULL,
  description TEXT NULL,
  start_date DATE NULL,
  target_date DATE NULL,
  archived TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  UNIQUE KEY project_workspace_code (workspace_id, code),
  CONSTRAINT project_workspace_fk FOREIGN KEY (workspace_id) REFERENCES workspace (id) ON DELETE CASCADE,
  CONSTRAINT project_date_order CHECK (target_date IS NULL OR start_date IS NULL OR target_date >= start_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE project_member (
  project_id BIGINT UNSIGNED NOT NULL,
  member_id BIGINT UNSIGNED NOT NULL,
  role ENUM('owner', 'editor', 'viewer') NOT NULL DEFAULT 'editor',
  PRIMARY KEY (project_id, member_id),
  KEY project_member_member (member_id),
  CONSTRAINT project_member_project_fk FOREIGN KEY (project_id) REFERENCES project (id) ON DELETE CASCADE,
  CONSTRAINT project_member_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE milestone (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(180) NOT NULL,
  due_date DATE NULL,
  completed_at DATETIME NULL,
  PRIMARY KEY (id),
  KEY milestone_project (project_id),
  CONSTRAINT milestone_project_fk FOREIGN KEY (project_id) REFERENCES project (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE project_status (
  project_id BIGINT UNSIGNED NOT NULL,
  code VARCHAR(32) NOT NULL,
  name VARCHAR(80) NOT NULL,
  position SMALLINT UNSIGNED NOT NULL,
  is_complete TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (project_id, code),
  CONSTRAINT project_status_project_fk FOREIGN KEY (project_id) REFERENCES project (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE label (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  workspace_id BIGINT UNSIGNED NOT NULL,
  name VARCHAR(80) NOT NULL,
  color CHAR(7) NOT NULL DEFAULT '#64748b',
  PRIMARY KEY (id),
  UNIQUE KEY label_workspace_name (workspace_id, name),
  CONSTRAINT label_workspace_fk FOREIGN KEY (workspace_id) REFERENCES workspace (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE project_label (
  project_id BIGINT UNSIGNED NOT NULL,
  label_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (project_id, label_id),
  KEY project_label_label (label_id),
  CONSTRAINT project_label_project_fk FOREIGN KEY (project_id) REFERENCES project (id) ON DELETE CASCADE,
  CONSTRAINT project_label_label_fk FOREIGN KEY (label_id) REFERENCES label (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE task (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  status_code VARCHAR(32) NOT NULL,
  title VARCHAR(240) NOT NULL,
  description MEDIUMTEXT NULL,
  parent_task_id BIGINT UNSIGNED NULL,
  milestone_id BIGINT UNSIGNED NULL,
  priority ENUM('low', 'normal', 'high', 'urgent') NOT NULL DEFAULT 'normal',
  estimate_hours DECIMAL(7,2) NULL,
  due_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY task_project_id (project_id, id),
  KEY task_project_status (project_id, status_code),
  KEY task_parent (parent_task_id),
  KEY task_milestone (milestone_id),
  CONSTRAINT task_project_fk FOREIGN KEY (project_id) REFERENCES project (id) ON DELETE CASCADE,
  CONSTRAINT task_status_fk FOREIGN KEY (project_id, status_code) REFERENCES project_status (project_id, code) ON DELETE RESTRICT,
  CONSTRAINT task_parent_fk FOREIGN KEY (parent_task_id) REFERENCES task (id) ON DELETE SET NULL,
  CONSTRAINT task_milestone_fk FOREIGN KEY (milestone_id) REFERENCES milestone (id) ON DELETE SET NULL,
  CONSTRAINT task_estimate_nonnegative CHECK (estimate_hours >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE task_dependency (
  task_id BIGINT UNSIGNED NOT NULL,
  depends_on_task_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (task_id, depends_on_task_id),
  KEY task_dependency_dependency (depends_on_task_id),
  CONSTRAINT task_dependency_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE,
  CONSTRAINT task_dependency_dependency_fk FOREIGN KEY (depends_on_task_id) REFERENCES task (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE task_assignee (
  task_id BIGINT UNSIGNED NOT NULL,
  member_id BIGINT UNSIGNED NOT NULL,
  assigned_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (task_id, member_id),
  KEY task_assignee_member (member_id),
  CONSTRAINT task_assignee_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE,
  CONSTRAINT task_assignee_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE task_label (
  project_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NOT NULL,
  label_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (project_id, task_id, label_id),
  KEY task_label_project_label (project_id, label_id),
  CONSTRAINT task_label_task_fk FOREIGN KEY (project_id, task_id) REFERENCES task (project_id, id) ON DELETE CASCADE,
  CONSTRAINT task_label_label_fk FOREIGN KEY (project_id, label_id) REFERENCES project_label (project_id, label_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE task_checklist (
  task_id BIGINT UNSIGNED NOT NULL,
  id INT UNSIGNED NOT NULL,
  title VARCHAR(160) NOT NULL,
  position SMALLINT UNSIGNED NOT NULL DEFAULT 0,
  PRIMARY KEY (task_id, id),
  CONSTRAINT task_checklist_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE checklist_item (
  task_id BIGINT UNSIGNED NOT NULL,
  checklist_id INT UNSIGNED NOT NULL,
  item_number INT UNSIGNED NOT NULL,
  title VARCHAR(240) NOT NULL,
  completed TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (task_id, checklist_id, item_number),
  CONSTRAINT checklist_item_checklist_fk FOREIGN KEY (task_id, checklist_id) REFERENCES task_checklist (task_id, id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE comment (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  task_id BIGINT UNSIGNED NOT NULL,
  author_id BIGINT UNSIGNED NULL,
  parent_comment_id BIGINT UNSIGNED NULL,
  body TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  edited_at DATETIME NULL,
  PRIMARY KEY (id),
  KEY comment_task (task_id),
  KEY comment_author (author_id),
  KEY comment_parent (parent_comment_id),
  CONSTRAINT comment_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE,
  CONSTRAINT comment_author_fk FOREIGN KEY (author_id) REFERENCES member (id) ON DELETE SET NULL,
  CONSTRAINT comment_parent_fk FOREIGN KEY (parent_comment_id) REFERENCES comment (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE comment_reaction (
  comment_id BIGINT UNSIGNED NOT NULL,
  member_id BIGINT UNSIGNED NOT NULL,
  reaction VARCHAR(32) NOT NULL,
  PRIMARY KEY (comment_id, member_id, reaction),
  KEY comment_reaction_member (member_id),
  CONSTRAINT comment_reaction_comment_fk FOREIGN KEY (comment_id) REFERENCES comment (id) ON DELETE CASCADE,
  CONSTRAINT comment_reaction_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE document (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  project_id BIGINT UNSIGNED NOT NULL,
  title VARCHAR(240) NOT NULL,
  body LONGTEXT NOT NULL,
  created_by BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY document_project (project_id),
  KEY document_author (created_by),
  FULLTEXT KEY document_search (title, body),
  CONSTRAINT document_project_fk FOREIGN KEY (project_id) REFERENCES project (id) ON DELETE CASCADE,
  CONSTRAINT document_author_fk FOREIGN KEY (created_by) REFERENCES member (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE document_revision (
  document_id BIGINT UNSIGNED NOT NULL,
  revision_number INT UNSIGNED NOT NULL,
  body LONGTEXT NOT NULL,
  author_id BIGINT UNSIGNED NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (document_id, revision_number),
  KEY document_revision_author (author_id),
  CONSTRAINT document_revision_document_fk FOREIGN KEY (document_id) REFERENCES document (id) ON DELETE CASCADE,
  CONSTRAINT document_revision_author_fk FOREIGN KEY (author_id) REFERENCES member (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE attachment (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  task_id BIGINT UNSIGNED NULL,
  document_id BIGINT UNSIGNED NULL,
  uploaded_by BIGINT UNSIGNED NULL,
  file_name VARCHAR(255) NOT NULL,
  mime_type VARCHAR(127) NOT NULL,
  size_bytes BIGINT UNSIGNED NOT NULL,
  checksum BINARY(32) NOT NULL,
  storage_key VARCHAR(1024) NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY attachment_task (task_id),
  KEY attachment_document (document_id),
  KEY attachment_uploader (uploaded_by),
  KEY attachment_storage (storage_key(191)),
  CONSTRAINT attachment_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE,
  CONSTRAINT attachment_document_fk FOREIGN KEY (document_id) REFERENCES document (id) ON DELETE CASCADE,
  CONSTRAINT attachment_uploader_fk FOREIGN KEY (uploaded_by) REFERENCES member (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE time_entry (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  task_id BIGINT UNSIGNED NOT NULL,
  member_id BIGINT UNSIGNED NOT NULL,
  started_at DATETIME NOT NULL,
  ended_at DATETIME NULL,
  notes TEXT NULL,
  billable TINYINT(1) NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY time_entry_task (task_id),
  KEY time_entry_member (member_id),
  CONSTRAINT time_entry_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE,
  CONSTRAINT time_entry_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE,
  CONSTRAINT time_entry_time_order CHECK (ended_at IS NULL OR ended_at >= started_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE timer (
  member_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NOT NULL,
  started_at DATETIME NOT NULL,
  PRIMARY KEY (member_id),
  KEY timer_task (task_id),
  CONSTRAINT timer_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE,
  CONSTRAINT timer_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE activity (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  workspace_id BIGINT UNSIGNED NOT NULL,
  actor_id BIGINT UNSIGNED NULL,
  task_id BIGINT UNSIGNED NULL,
  event_type VARCHAR(64) NOT NULL,
  payload JSON NOT NULL,
  occurred_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY activity_workspace_time (workspace_id, occurred_at DESC),
  KEY activity_actor (actor_id),
  KEY activity_task (task_id),
  CONSTRAINT activity_workspace_fk FOREIGN KEY (workspace_id) REFERENCES workspace (id) ON DELETE CASCADE,
  CONSTRAINT activity_actor_fk FOREIGN KEY (actor_id) REFERENCES member (id) ON DELETE SET NULL,
  CONSTRAINT activity_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE notification (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  member_id BIGINT UNSIGNED NOT NULL,
  task_id BIGINT UNSIGNED NULL,
  channel ENUM('in_app', 'email') NOT NULL DEFAULT 'in_app',
  payload JSON NOT NULL,
  read_at DATETIME NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY notification_member_read (member_id, read_at),
  KEY notification_task (task_id),
  CONSTRAINT notification_member_fk FOREIGN KEY (member_id) REFERENCES member (id) ON DELETE CASCADE,
  CONSTRAINT notification_task_fk FOREIGN KEY (task_id) REFERENCES task (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE webhook (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  workspace_id BIGINT UNSIGNED NOT NULL,
  url VARCHAR(2048) NOT NULL,
  events JSON NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY webhook_workspace (workspace_id),
  CONSTRAINT webhook_workspace_fk FOREIGN KEY (workspace_id) REFERENCES workspace (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE webhook_delivery (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  webhook_id BIGINT UNSIGNED NOT NULL,
  event_id BINARY(16) NOT NULL,
  attempt SMALLINT UNSIGNED NOT NULL DEFAULT 1,
  status_code SMALLINT UNSIGNED NULL,
  response_body MEDIUMTEXT NULL,
  attempted_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  duration_ms INT UNSIGNED NULL,
  PRIMARY KEY (id),
  UNIQUE KEY webhook_delivery_event_attempt (webhook_id, event_id, attempt),
  CONSTRAINT webhook_delivery_webhook_fk FOREIGN KEY (webhook_id) REFERENCES webhook (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
