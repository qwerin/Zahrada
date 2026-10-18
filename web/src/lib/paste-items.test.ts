import { describe, expect, it } from 'vitest'
import { parsePastedItems } from './paste-items'

describe('parsePastedItems', () => {
  it('markdown tabulka z chatu', () => {
    const items = parsePastedItems(`## Příprava terénu
| Položka | Množství | Jednotka | Cena za j. | Celkem |
|---|---|---|---|---|
| Frézování půdy | 250 | m² | 15 Kč | 3 750 Kč |
| **Odvoz kamení** | 2 | fůra | 1 200 Kč | 2 400 Kč |
| **Celkem** | | | | 6 150 Kč |

## Trávník
| Položka | Množství | Cena celkem |
|---|---|---|
| Travní osivo | 7,5 kg | 1 755 Kč |`)
    expect(items).toEqual([
      { section: 'Příprava terénu', name: 'Frézování půdy', quantity: '250', unit: 'm²', unitPrice: 1500, kind: 'work' },
      { section: 'Příprava terénu', name: 'Odvoz kamení', quantity: '2', unit: 'fůra', unitPrice: 120000, kind: 'transport' },
      { section: 'Trávník', name: 'Travní osivo', quantity: '7.5', unit: 'kg', unitPrice: 23400, kind: 'material' },
    ])
  })

  it('Excel (tabulátory) bez hlavičky a volné řádky', () => {
    const items = parsePastedItems(
      'Výsadba keře\t24\tks\t180\nTrávníkový substrát\t12,5\tm3\t1290\n\nPokládka travního koberce 120 m² × 95 Kč\n- Mulčovací kůra: 3 m³ á 1 450 Kč',
    )
    expect(items.map((i) => [i.name, i.quantity, i.unit, i.unitPrice, i.kind])).toEqual([
      ['Výsadba keře', '24', 'ks', 18000, 'work'],
      ['Trávníkový substrát', '12.5', 'm³', 129000, 'material'],
      ['Pokládka travního koberce', '120', 'm²', 9500, 'work'],
      ['Mulčovací kůra', '3', 'm³', 145000, 'material'],
    ])
  })
})
