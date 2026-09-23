import { useState } from 'react'
import { api, describeError } from '../api'
import { useToast } from './Toast'

interface Props {
  open: boolean
  onClose: () => void
  onUploaded: () => void
}

export function UploadDialog({ open, onClose, onUploaded }: Props) {
  const toast = useToast()
  const [progress, setProgress] = useState(0)
  const [busy, setBusy] = useState(false)

  if (!open) return null

  async function handleFile(file: File) {
    setBusy(true)
    setProgress(0)
    try {
      await api.upload(file, setProgress)
      toast('上传成功，已写入材料与知识库正文', 'success')
      onUploaded()
      onClose()
    } catch (error) {
      toast(describeError(error), 'error')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="palette-backdrop" onClick={busy ? undefined : onClose}>
      <div className="dialog" onClick={(event) => event.stopPropagation()}>
        <h2>上传教研材料</h2>
        <p className="muted">支持 .txt 与 .md，内容须为非空 UTF-8 文本。</p>
        <input
          type="file"
          accept=".txt,.md,text/plain,text/markdown"
          disabled={busy}
          onChange={(event) => {
            const file = event.target.files?.[0]
            if (file) void handleFile(file)
          }}
        />
        {busy ? (
          <div className="progress" aria-label="上传进度">
            <div className="progress-bar" style={{ width: `${progress}%` }} />
            <span>{progress}%</span>
          </div>
        ) : null}
      </div>
    </div>
  )
}
