// Разбор строки с комбинацией вида «02, 19, 34, 07, 26, 03, 14 и 48».
// Разделителем считается любая подстрока без цифр: запятые, пробелы, «и» и т.п.
// Возвращает { numbers: [7 чисел по возрастанию], bonus } или { error: 'сообщение' }.
export function parseCombination(input) {
  const parts = String(input).split(/[^0-9]+/).filter((p) => p !== '');
  const nums = parts.map(Number);
  if (nums.length !== 8) {
    return { error: `Нужно ровно 8 чисел (7 основных и 1 бонус), найдено: ${nums.length}` };
  }
  const main = nums.slice(0, 7);
  const seen = new Set();
  for (const n of main) {
    if (n < 1 || n > 35) {
      return { error: `Число ${n} вне диапазона 1–35` };
    }
    if (seen.has(n)) {
      return { error: `Число ${n} повторяется — все семь должны быть разными` };
    }
    seen.add(n);
  }
  const bonus = nums[7];
  if (bonus < 1 || bonus > 54) {
    return { error: `Бонусное число ${bonus} вне диапазона 1–54` };
  }
  return { numbers: main.sort((a, b) => a - b), bonus };
}
