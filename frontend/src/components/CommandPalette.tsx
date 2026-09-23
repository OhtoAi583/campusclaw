import { useEffect, useMemo, useState } from 'react'
import type { Material } from '../types'

export interface Command {
  label: string
  hint?: string
  run: () => void
}

interface Props {
  open: boolean
  onClose: () => void
  materials: Material[]
  commands: Command[]
  onOpenMaterial: (material: Material) => void
}

export function CommandPalette({ open, onClose, materials, commands, onOpenMaterial }: Props) {
  const [query, setQuery] = useState('')

  useEffect(() => {
    if (open) setQuery('')
  }, [open])

  const items = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const materialItems: Command[] = materials
      .filter((material) => !keyword || material.title.toLowerCase().includes(keyword))
      .slice(0, 6)
      .map((material) => ({
        label: material.title,
        hint: '打开材料',
        run: () => onOpenMaterial(material),
      }))
    const commandItems = commands.filter((command) => !keyword || command.label.toLowerCase().includes(keyword))
    return [...materialItems, ...commandItems]
  }, [query, materials, commands, onOpenMaterial])

  if (!open) return null

  return (
    <div className="palette-backdrop" onClick={onClose}>
      <div className="palette" onClick={(event) => event.stopPropagation()}>
        <input
          autoFocus
          value={query}
          placeholder="搜索本班材料或执行命令…"
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Escape') onClose()
            if (event.key === 'Enter' && items[0]) {
              items[0].run()
              onClose()
            }
          }}
        />
        <ul>
          {items.map((item, index) => (
            <li key={`${item.label}-${index}`}>
              <button
                type="button"
                onClick={() => {
                  item.run()
                  onClose()
                }}
              >
                <span>{item.label}</span>
                {item.hint ? <small>{item.hint}</small> : null}
              </button>
            </li>
          ))}
          {items.length === 0 ? <li className="palette-empty">没有匹配项</li> : null}
        </ul>
      </div>
    </div>
  )
}
