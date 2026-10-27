"""Скачивает все существительные из 5 букв со словаря nskhuman.ru/unislov.

Сайт делит слова по первой букве (nlet1=1..33, 1 = А, 33 = Я), а внутри
буквы — ещё и по второй букве (nlet2). Страница nlet1=N без nlet2 показывает
только первую подгруппу, поэтому скрипт берёт с неё ссылки на остальные
подгруппы и обходит их все.

Запуск:
    python3 parse_nskhuman.py [выходной_файл]   (по умолчанию words5_nskhuman.txt)
"""

import re
import sys
import time
import urllib.error
import urllib.request

BASE = "http://nskhuman.ru/unislov/suschestv.php"
DELAY = 0.3  # пауза между запросами, чтобы не нагружать сайт
# Ссылка на слово; у букв без подгрупп в ссылке нет "&nchast=1". Звёздочка
# после ссылки на сайте означает «стилистически окрашенное или устаревшее
# слово» — такие слова для игры не подходят.
WORD_RE = re.compile(r'<a href="slovar\.php\?nslovo=\d+[^"]*"\s*>\s*([^<]+?)\s*</a>\s*(\*)?', re.I)
FIVE_LETTERS = re.compile(r"^[а-яё]{5}$")


def fetch(query: str) -> str:
    url = f"{BASE}?{query}"
    for attempt in range(3):
        try:
            with urllib.request.urlopen(url, timeout=30) as resp:
                return resp.read().decode("windows-1251", errors="replace")
        except (urllib.error.URLError, TimeoutError) as e:
            print(f"  ошибка {url}: {e}, повтор {attempt + 1}/3", file=sys.stderr)
            time.sleep(2)
    raise RuntimeError(f"не удалось скачать {url}")


def subgroup_links(page: str, nlet1: int) -> list[int]:
    """Номера подгрупп nlet2, на которые ссылается страница буквы nlet1."""
    found = re.findall(rf'suschestv\.php\?nlet1={nlet1}&nlet2=(\d+)"', page, re.I)
    return sorted({int(n) for n in found})


def main() -> None:
    out_path = sys.argv[1] if len(sys.argv) > 1 else "words5_nskhuman.txt"
    words: set[str] = set()
    skipped_capital = set()
    skipped_starred = set()

    def collect(page: str) -> None:
        for w, star in WORD_RE.findall(page):
            if not FIVE_LETTERS.match(w):
                if FIVE_LETTERS.match(w.lower()):
                    skipped_capital.add(w)  # имена собственные: Покров, Литва...
            elif star:
                skipped_starred.add(w)
            else:
                words.add(w)

    for nlet1 in range(1, 34):
        first = fetch(f"nlet1={nlet1}")
        time.sleep(DELAY)
        if not WORD_RE.search(first):
            continue  # Ъ, Ы, Ь — слов на эти буквы нет
        collect(first)
        subs = subgroup_links(first, nlet1)
        for nlet2 in subs:
            collect(fetch(f"nlet1={nlet1}&nlet2={nlet2}"))
            time.sleep(DELAY)
        print(f"буква {nlet1:2}: подгрупп {len(subs) + 1:2}, всего слов из 5 букв: {len(words)}")

    with open(out_path, "w", encoding="utf-8") as f:
        f.write("\n".join(sorted(words)) + "\n")

    print(f"\nГотово: {len(words)} слов записано в {out_path}")
    # Слово может быть на сайте дважды (разные значения): со звёздочкой и без.
    # Тогда оно годится и остаётся в списке.
    starred_only = skipped_starred - words
    print(f"Пропущено слов со звёздочкой (устаревшие/стилистически окрашенные): {len(starred_only)}")
    if skipped_capital:
        print(f"Пропущено слов с заглавной буквы (имена собственные): {len(skipped_capital)}")


if __name__ == "__main__":
    main()
