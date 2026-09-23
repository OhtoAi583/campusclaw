export type Role = 'teacher' | 'student'

export interface Identity {
  user_id: number
  username: string
  role: Role
  class_id: number
  class_name: string
}

export interface Material {
  id: number
  class_id: number
  title: string
  original_name: string
  size_bytes: number
  uploaded_by: number
  uploader_name: string
  created_at: string
  content?: string
}
