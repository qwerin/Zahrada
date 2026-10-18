import { ClipboardPasteIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { parsePastedItems, type PastedItem } from '@/lib/paste-items'
import { formatMoney } from '@/lib/money'

const PROMPT_TIP =
  'Tip pro Claude: „Napiš položky jako tabulku se sloupci Položka | Množství | Jednotka | Cena za j. (Kč), oddíly jako nadpisy.“'

/** Vložení položek z textu (chat s Claude, Excel, e-mail). Vrací rozpoznané položky. */
export function PasteItemsDialog({
  open,
  onOpenChange,
  onImport,
  title = 'Vložit položky z textu',
  busy,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  onImport: (items: PastedItem[]) => void
  title?: string
  busy?: boolean
}) {
  const [text, setText] = useState('')
  const items = useMemo(() => parsePastedItems(text), [text])
  const pasteFromClipboard = async () => {
    try {
      setText(await navigator.clipboard.readText())
    } catch {
      /* prohlížeč nepovolil — uživatel vloží ručně */
    }
  }
  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description="Vložte tabulku nebo seznam položek, například odpověď z chatu s Claude nebo řádky zkopírované z Excelu."
      footer={
        <Button disabled={items.length === 0 || busy} onClick={() => { onImport(items); setText('') }}>
          Vložit {items.length > 0 ? `${items.length} položek` : ''}
        </Button>
      }
    >
      <div className="grid gap-3">
        <Button variant="outline" size="sm" className="justify-self-start" onClick={() => void pasteFromClipboard()}>
          <ClipboardPasteIcon data-icon="inline-start" /> Vložit ze schránky
        </Button>
        <Textarea
          rows={8}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={'| Položka | Množství | Jednotka | Cena za j. |\n|---|---|---|---|\n| Frézování půdy | 250 | m² | 15 Kč |\n\nnebo: Výsadba keře 24 ks × 180 Kč'}
          className="font-mono text-xs"
        />
        <p className="text-xs text-muted-foreground">{PROMPT_TIP}</p>
        {items.length > 0 && (
          <div className="max-h-56 overflow-auto rounded-md border text-sm">
            {items.map((i, n) => (
              <div key={n} className="flex gap-2 border-b px-2 py-1 last:border-b-0">
                <span className="min-w-0 flex-1 truncate">
                  {i.section && <span className="text-muted-foreground">{i.section} · </span>}
                  {i.name}
                </span>
                <span className="tabular-nums">{i.quantity.replace('.', ',')} {i.unit}</span>
                <span className="w-24 text-right tabular-nums">{i.unitPrice !== undefined ? formatMoney(i.unitPrice) : '–'}</span>
              </div>
            ))}
          </div>
        )}
        {text.trim() && items.length === 0 && (
          <p className="text-sm text-destructive">Nenašel jsem žádné položky. Zkuste tabulku nebo řádky „název množství jednotka × cena“.</p>
        )}
      </div>
    </ResponsiveDialog>
  )
}
