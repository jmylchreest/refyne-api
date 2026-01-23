/**
 * Seeded random number generator using a simple LCG algorithm.
 * Returns a function that generates numbers between 0 and 1.
 */
function createSeededRandom(seed: number): () => number {
  let state = seed;
  return () => {
    // Linear Congruential Generator parameters (same as glibc)
    state = (state * 1103515245 + 12345) & 0x7fffffff;
    return state / 0x7fffffff;
  };
}

/**
 * Get a seed based on the current date (changes daily).
 * This ensures consistent ordering within a single build,
 * but different ordering each day.
 */
function getDailySeed(): number {
  const now = new Date();
  // Use year, month, day to create a daily seed
  return now.getFullYear() * 10000 + (now.getMonth() + 1) * 100 + now.getDate();
}

/**
 * Fisher-Yates shuffle with a seeded random generator.
 * Creates a new shuffled array without modifying the original.
 */
export function shuffleWithDailySeed<T>(array: T[]): T[] {
  const result = [...array];
  const random = createSeededRandom(getDailySeed());

  for (let i = result.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [result[i], result[j]] = [result[j], result[i]];
  }

  return result;
}
