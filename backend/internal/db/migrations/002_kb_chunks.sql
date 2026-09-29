-- 迭代 2：知识库检索的块表。
-- 块是检索与溯源的最小单位：带字符区间（start_offset/end_offset）以便回到正文核对。
-- class_id 与 materials 保持一致（非空 + 索引），保证按班级召回在 SQL 层就完成过滤。

CREATE TABLE IF NOT EXISTS kb_chunks (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  material_id   BIGINT UNSIGNED NOT NULL,
  class_id      BIGINT UNSIGNED NOT NULL,
  chunk_index   INT UNSIGNED    NOT NULL,
  start_offset  INT UNSIGNED    NOT NULL,
  end_offset    INT UNSIGNED    NOT NULL,
  content       MEDIUMTEXT      NOT NULL,
  embedding     BLOB            NOT NULL,
  embedding_dim INT UNSIGNED    NOT NULL,
  created_at    DATETIME        NOT NULL DEFAULT (UTC_TIMESTAMP()),
  PRIMARY KEY (id),
  UNIQUE KEY uk_kb_chunks_material_index (material_id, chunk_index),
  KEY idx_kb_chunks_class_id (class_id),
  KEY idx_kb_chunks_material_id (material_id),
  CONSTRAINT fk_kb_chunks_material FOREIGN KEY (material_id) REFERENCES materials (id) ON DELETE CASCADE,
  CONSTRAINT fk_kb_chunks_class FOREIGN KEY (class_id) REFERENCES classes (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
