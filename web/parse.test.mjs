import test from 'node:test';
import assert from 'node:assert/strict';
import { parseCombination } from './parse.js';

test('пример из ТЗ: запятые и «и»', () => {
  assert.deepEqual(
    parseCombination('02, 19, 34, 07, 26, 03, 14 и 48'),
    { numbers: [2, 3, 7, 14, 19, 26, 34], bonus: 48 },
  );
});

test('разделитель — просто пробелы', () => {
  assert.deepEqual(
    parseCombination('2 3 7 14 19 26 34 48'),
    { numbers: [2, 3, 7, 14, 19, 26, 34], bonus: 48 },
  );
});

test('ведущие нули', () => {
  assert.deepEqual(
    parseCombination('01,02,03,04,05,06,07,54'),
    { numbers: [1, 2, 3, 4, 5, 6, 7], bonus: 54 },
  );
});

test('мало чисел — ошибка', () => {
  assert.match(parseCombination('1 2 3').error, /ровно 8 чисел/);
});

test('повтор в семёрке — ошибка', () => {
  assert.match(parseCombination('5 5 2 3 4 6 7 8').error, /повторяется/);
});

test('основное число вне 1–35 — ошибка', () => {
  assert.match(parseCombination('1 2 3 4 5 6 36 8').error, /вне диапазона 1–35/);
});

test('бонус вне 1–54 — ошибка', () => {
  assert.match(parseCombination('1 2 3 4 5 6 7 55').error, /вне диапазона 1–54/);
});

test('нечисловая строка — ошибка «мало чисел»', () => {
  assert.match(parseCombination('привет').error, /ровно 8 чисел/);
});
