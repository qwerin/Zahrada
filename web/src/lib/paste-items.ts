// Převod vloženého textu na položky kalkulace / ceníku.
// Umí: markdown tabulku (z chatu s Claude), řádky z Excelu (tabulátory), CSV se středníky
// a volné řádky typu „Frézování půdy 250 m² × 15 Kč“. Nadpisy („## Trávník“, „Trávník:“) jsou oddíly.

export type ItemKind = 'work' | 'material' | 'machine' | 'transport' | 'subcontract' | 'other'

export interface PastedItem {
  section: string
  name: string
  quantity: string // desetinný řetězec s tečkou
  unit: string
  unitPrice?: number // haléře za jednotku (prodejní), když jde z textu poznat
  kind: ItemKind
}

const UNIT_RE = '(m²|m2|m³|m3|bm|ks|kus(?:ů|y)?|kg|t|tun(?:a|y)?|hod(?:in(?:a|y)?)?|h|km|fůr(?:a|y)?|kpl|komplet|m|l|bal(?:ení)?|sáček|pytl(?:e|ů)?)'

const UNIT_NORMAL: Record<string, string> = {
  m2: 'm²', m3: 'm³', kus: 'ks', kusy: 'ks', kusů: 'ks', tuna: 't', tuny: 't', tun: 't', hodina: 'hod', hodiny: 'hod',
  hodin: 'hod', h: 'hod', fůra: 'fůra', fůry: 'fůra', komplet: 'kpl', balení: 'bal', pytle: 'pytel', pytlů: 'pytel',
}

export function normalizeUnit(u: string): string {
  const v = u.trim().toLowerCase().replace(/\.$/, '')
  return UNIT_NORMAL[v] ?? v
}

/** „1 250,50“ / „1250.5“ / „15,-“ → 1250.5; nečíslo → NaN. */
export function parseCzNumber(s: string): number {
  const v = s.replace(/\s|\u00a0/g, '').replace(/,-$/, '').replace(/Kč|CZK|,-/gi, '')
  if (!/^-?\d+([.,]\d+)?$/.test(v)) return NaN
  return Number(v.replace(',', '.'))
}

const WORK_WORDS = /(práce|montáž|výsadb|sázen|sečen|frézo|pokládk|úprav|výkop|stříhán|prořez|řez|odstraněn|instalac|založen|hnojen|vertikut|aerac|úklid|hodinov|výsev|zaválcov|mulčován|rytí|plení)/i
const TRANSPORT_WORDS = /(doprav|odvoz|přeprav|dovoz|cestovn|km\b)/i
const MACHINE_WORDS = /(bagr|fréz[ay] |stroj|mechaniz|nakladač|štěpkovač|pronájem)/i

export function guessKind(name: string, unit: string): ItemKind {
  if (TRANSPORT_WORDS.test(name) || unit === 'km' || unit === 'fůra') return 'transport'
  if (MACHINE_WORDS.test(name)) return 'machine'
  if (unit === 'hod' || WORK_WORDS.test(name)) return 'work'
  return 'material'
}

const HEADER_ALIASES: Record<string, keyof PastedItem | 'price' | 'total' | 'skip'> = {
  položka: 'name', název: 'name', popis: 'name', práce: 'name', materiál: 'name', 'položka / práce': 'name',
  množství: 'quantity', mn: 'quantity', počet: 'quantity', 'mn.': 'quantity', ks: 'quantity',
  jednotka: 'unit', mj: 'unit', 'm.j.': 'unit', 'j.': 'unit', jedn: 'unit', 'jedn.': 'unit',
  cena: 'price', 'cena/j': 'price', 'cena za j.': 'price', 'cena za jednotku': 'price', 'jedn. cena': 'price',
  'jednotková cena': 'price', 'kč/j': 'price', 'cena/mj': 'price', sazba: 'price',
  celkem: 'total', 'cena celkem': 'total', součet: 'total', 'celkem kč': 'total',
  oddíl: 'section', sekce: 'section', kategorie: 'section', '#': 'skip', 'č.': 'skip',
}

function headerKey(cell: string) {
  const v = cell.trim().toLowerCase().replace(/\*|_/g, '').replace(/\s*\(.*\)\s*/, '').trim()
  return HEADER_ALIASES[v] ?? (v.startsWith('cena') && v.includes('celk') ? 'total' : v.startsWith('cena') ? 'price' : undefined)
}

function splitRow(line: string): string[] | null {
  const t = line.trim()
  if (t.startsWith('|')) return t.replace(/^\||\|$/g, '').split('|').map((c) => c.trim())
  if (t.includes('\t')) return t.split('\t').map((c) => c.trim())
  if ((t.match(/;/g) ?? []).length >= 2) return t.split(';').map((c) => c.trim())
  return null
}

function stripMd(s: string) {
  return s.replace(/\*\*|__|`/g, '').replace(/^[-*•]\s+/, '').replace(/^\d+[.)]\s+/, '').trim()
}

/** Množství s jednotkou uvnitř buňky („250 m²“). */
function qtyUnitFromCell(cell: string): { q?: string; u?: string } {
  const m = new RegExp(`^(\\d[\\d\\s]*(?:[.,]\\d+)?)\\s*${UNIT_RE}?\\.?$`, 'i').exec(cell.trim())
  if (!m) return {}
  return { q: String(parseCzNumber(m[1]!)), u: m[2] ? normalizeUnit(m[2]) : undefined }
}

function fromTable(rows: string[][], section: string, out: PastedItem[]) {
  let header: ReturnType<typeof headerKey>[] | null = null
  for (const cells of rows) {
    if (cells.every((c) => /^:?-{2,}:?$/.test(c) || c === '')) continue // oddělovač markdown tabulky
    const keys = cells.map(headerKey)
    if (!header && keys.filter(Boolean).length >= 2 && keys.includes('name')) {
      header = keys
      continue
    }
    const get = (k: string) => (header ? cells[header.indexOf(k as never)] : undefined)
    let name = stripMd(header ? (get('name') ?? '') : (cells[0] ?? ''))
    if (!name || /^(celkem|součet|mezisoučet|dph|cena celkem)/i.test(name)) continue
    let quantity = '1'
    let unit = ''
    let price: number | undefined
    if (header) {
      const qu = qtyUnitFromCell(get('quantity') ?? '')
      if (qu.q) quantity = qu.q
      unit = normalizeUnit(get('unit') ?? '') || qu.u || ''
      const p = parseCzNumber(get('price') ?? '')
      if (!Number.isNaN(p)) price = Math.round(p * 100)
      else {
        const total = parseCzNumber(get('total') ?? '')
        if (!Number.isNaN(total) && Number(quantity) > 0) price = Math.round((total / Number(quantity)) * 100)
      }
      const sec = get('section')
      if (sec) section = stripMd(sec)
    } else {
      // bez hlavičky: název, množství, jednotka, cena (co z toho je)
      const qu = qtyUnitFromCell(cells[1] ?? '')
      if (qu.q) quantity = qu.q
      unit = qu.u ?? normalizeUnit(cells[2] ?? '')
      const p = parseCzNumber(cells[qu.u ? 2 : 3] ?? '')
      if (!Number.isNaN(p)) price = Math.round(p * 100)
    }
    name = name.replace(/\s{2,}/g, ' ')
    out.push({ section, name, quantity, unit, unitPrice: price, kind: guessKind(name, unit) })
  }
}

const FREE_RE = new RegExp(
  `^(.+?)[\\s:–-]+(\\d[\\d\\s]*(?:[.,]\\d+)?)\\s*${UNIT_RE}\\.?(?:\\s*(?:×|x|\\*|á|à|po|@|a)\\s*(\\d[\\d\\s]*(?:[.,]\\d+)?)\\s*(?:Kč|,-|CZK)?(?:\\s*/\\s*\\S+)?)?`,
  'i',
)

/** Hlavní funkce: text → položky. */
export function parsePastedItems(text: string): PastedItem[] {
  const out: PastedItem[] = []
  let section = ''
  const lines = text.replace(/\r/g, '').split('\n')
  let table: string[][] = []
  const flush = () => {
    if (table.length) fromTable(table, section, out)
    table = []
  }
  for (const raw of lines) {
    const line = raw.trim()
    if (!line) {
      flush()
      continue
    }
    const cells = splitRow(line)
    if (cells) {
      table.push(cells)
      continue
    }
    flush()
    const heading = /^#{1,6}\s+(.+)$/.exec(line) ?? /^\*\*(.+)\*\*:?$/.exec(line) ?? /^([^\d:]{2,60}):$/.exec(line)
    if (heading) {
      section = stripMd(heading[1]!).replace(/:$/, '')
      continue
    }
    const m = FREE_RE.exec(stripMd(line))
    if (m) {
      const name = stripMd(m[1]!).replace(/[,:–-]\s*$/, '')
      const unit = normalizeUnit(m[3]!)
      const price = m[4] ? parseCzNumber(m[4]) : NaN
      out.push({
        section,
        name,
        quantity: String(parseCzNumber(m[2]!)),
        unit,
        unitPrice: Number.isNaN(price) ? undefined : Math.round(price * 100),
        kind: guessKind(name, unit),
      })
    }
  }
  flush()
  return out.filter((i) => i.name && Number.isFinite(Number(i.quantity)))
}
