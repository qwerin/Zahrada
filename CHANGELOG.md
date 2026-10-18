# Novinky v Zahradě

<!--
Tento soubor se zobrazuje přímo v aplikaci (sekce „Novinky“) — píše se pro uživatele, ne pro vývojáře.
Pravidla (viz CLAUDE.md → „Novinky“):
- Nejnovější záznam nahoře. Hlavička: `## RRRR-MM-DD · Krátký název` (datum vydání). Jedno datum = jeden záznam.
- Pod ní volitelně `### Podnadpis` a odrážky `- **Co** — k čemu je to dobré.` Česky, bez technického žargonu.
- Každá změna, kterou uživatel uvidí nebo pocítí, sem patří ve stejném commitu. Interní úpravy ne.
- Podporováno: odstavce, `###`, odrážky, **tučně**, *zvýraznění* (např. cesta v menu), `kód`, [odkazy](https://…). Nic dalšího.
-->

## 2026-10-18 · Kalkulace a nabídky, ceník, zálohy dodavatelům, dokumenty a AI asistent

### Kalkulace → nabídka
- **Kalkulace u zakázky** — v detailu zakázky klikněte na *Nová kalkulace*. Položky přidáte z ceníku, vložíte z textu, nebo napíšete ručně. Průběžně uvidíte cenu bez DPH i s DPH, náklady, marži a odhad hodin práce.
- **Z kalkulace nabídka** — *Odeslat klientovi* přepne zakázku na „nabídka odeslána“, *Přijato klientem* ji přepne na „objednáno“. Úpravy po odeslání děláte v *Nové verzi*, starší verze zůstanou pro přehled.
- **Tisk a PDF** — tlačítko *Tisk / PDF* otevře nabídku pro klienta s vaší hlavičkou a oddíly. Uložíte ji jako PDF přímo z prohlížeče. Náklady ani marže v ní nejsou.

### Ceník
- **Připravené práce a materiály** — v menu *Ceník* si uložte položky s nákupní i prodejní cenou, přirážkou, ztratným a pracností. Do kalkulace je pak přidáte jedním klepnutím.
- **Vložení z textu** — celý ceník nebo položky kalkulace vložíte najednou: tabulku z chatu s Claude, řádky z Excelu nebo řádky typu „Výsadba keře 24 ks × 180 Kč“. Nadpisy se stanou oddíly. Nejlépe funguje tabulka se sloupci *Položka | Množství | Jednotka | Cena za j.*

### Přiřazení dodavatelů k zakázkám
- **Karta Dodavatelé v zakázce** — hned nahoře v detailu zakázky vidíte, kdo na ní pracuje, kolik hodin už odpracoval, a můžete mu rovnou zavolat. Tlačítkem *Přiřadit* dodavatele zaškrtnete, křížkem odeberete.
- **Kdo je volný** — při přiřazování u každého dodavatele vidíte, na kolika rozpracovaných zakázkách už je.
- **Z druhé strany** — v detailu dodavatele jsou jeho rozpracované zakázky a tlačítko *Přiřadit k zakázce*.
- **Seznam zakázek** ukazuje přiřazené dodavatele a dá se filtrovat podle dodavatele.

### Dodavatelé bez e-mailu
- **Přidání dodavatele v týmu** — v *Nastavení → Tým* klikněte na *Přidat dodavatele*. E-mail je nepovinný: s e-mailem mu přijde pozvánka, bez e-mailu dostane přihlášení do telefonu.
- **Přihlášení QR kódem nebo odkazem** — dodavatel naskenuje QR kód fotoaparátem telefonu (nebo mu odkaz pošlete SMS či WhatsAppem) a je přihlášený, žádné heslo si nepamatuje. Odkaz funguje jednou a platí 7 dní. Přihlášení pak vydrží, dokud aplikaci používá.
- **Ztracený telefon** — v detailu dodavatele vytvořte *Nový přihlašovací odkaz*. Původní telefon se tím odhlásí.

### Zálohy dodavatelům
- **Vyplacení zálohy** — v detailu dodavatele klikněte na *Vyplatit zálohu* (datum, částka, hotově nebo převodem). Zálohu zapíšete i přímo v *Docházce* u souhrnu dodavatele.
- **Zůstatek vždy po ruce** — u dodavatele uvidíte, kolik odpracoval (jen schválená docházka), kolik už dostal a kolik mu zbývá doplatit. Tlačítko *Doplatit* předvyplní zbývající částku.
- **V měsíčním souhrnu docházky** je u každého dodavatele vidět záloha, výplata a částka k výplatě za měsíc. Dodavatel vidí své zálohy a zůstatek v *Moje práce*.
- **Uzavřený měsíc** — zálohy v uzavřeném období už nejde měnit ani mazat, stejně jako docházku.

### Dokumenty a fotky
- **Dokumenty k zakázce** — v detailu zakázky nahrajete smlouvy, výkresy, plány výsadby nebo rozpočty: PDF, Word, Excel, OpenDocument, TXT i CSV. Fotky mají vlastní sekci.
- **Fotky ze zakázek** — vedoucí je přidává v detailu zakázky, dodavatel v *Moje práce* u každé své zakázky (*Fotky z práce*). Tlačítko *Vyfotit* otevře na telefonu rovnou fotoaparát, *Z galerie* nahraje i víc už pořízených fotek najednou. U fotky je vidět, kdo ji nahrál a kdy.

### AI asistent (MCP)
- **Připojení Claude k Zahradě** — v *Nastavení → API tokeny* najdete adresu pro připojení AI asistenta. V claude.ai ji přidáte jako vlastní konektor a jen se přihlásíte, token není potřeba. Claude Code a další klienti použijí API token, příkaz i s ním vám po vytvoření tokenu nabídneme.
- **Co asistent umí** — najít zakázku, klienta nebo dodavatele, říct, kdo kolik odpracoval a co čeká na schválení, založit klienta se zahradou a zakázku, připravit kalkulaci z vašeho ceníku, doplnit ceník a zapsat docházku. Nic nemaže, neschvaluje, neposílá klientům a nemění nastavení. Smí jen to, co vy.
- **Připojené aplikace** — uvidíte a kdykoli odpojíte v *Nastavení → API tokeny → Připojené aplikace*. Změna hesla je odpojí všechny.

### Ostatní
- **Přestávka je předvyplněná na 0 minut** — při odchodu i při ručním zápisu docházky. Když přestávka byla, stačí ji dopsat.
- **Klienti na mobilu** — najdete je v menu *Více*. Nového klienta i se zahradou založíte rovnou při zakládání zakázky.
- **Novinky** — tato stránka. Když přibude něco nového, uvidíte u položky *Novinky* značku.

## 2026-10-17 · První verze: zakázky, klienti a docházka

- **Zakázky** — realizace, údržba i jednorázové práce s číslem, termínem, zahradou a přiřazenými dodavateli. Stav posunete tlačítky (poptávka → objednáno → v realizaci → dokončeno).
- **Klienti a zahrady** — kontakt na klienta a jeho zahrady s adresou, výměrami a poznámkou k přístupu (klíč, kód brány, pes).
- **Dodavatelé** — hodinová sazba a sazba za km. Dodavatele pozvete do aplikace a on pak vidí jen svou práci, žádné ceny pro klienty.
- **Docházka** — přehled po měsících, souhrn hodin a částek k proplacení po dodavatelích a zakázkách, hromadné schvalování, uzávěrka měsíce a export do Excelu.
- **Moje práce pro dodavatele** — velká tlačítka *Příchod* a *Odchod*, přidělené zakázky s adresou a odkazem do map a vlastní docházka, kterou pošle ke schválení.
