export interface Recipe {
  id: number;
  name: string;
  ingredients: string[];
  instructions: string[];
  prep_time_minutes: number;
  cook_time_minutes: number;
  total_time_minutes: number;
  servings: number;
  difficulty: 'Easy' | 'Medium' | 'Hard';
  cuisine: string;
  calories_per_serving: number;
  tags: string[];
  image: string;
  rating: number;
  review_count: number;
  meal_type: string[];
}

interface DummyJSONRecipe {
  id: number;
  name: string;
  ingredients: string[];
  instructions: string[];
  prepTimeMinutes: number;
  cookTimeMinutes: number;
  servings: number;
  difficulty: string;
  cuisine: string;
  caloriesPerServing: number;
  tags: string[];
  userId: number;
  image: string;
  rating: number;
  reviewCount: number;
  mealType: string[];
}

interface DummyJSONResponse {
  recipes: DummyJSONRecipe[];
  total: number;
}

let cachedRecipes: Recipe[] | null = null;

export async function getRecipes(): Promise<Recipe[]> {
  if (cachedRecipes) return cachedRecipes;

  try {
    const response = await fetch('https://dummyjson.com/recipes?limit=50');

    if (!response.ok) {
      throw new Error(`DummyJSON API error: ${response.status}`);
    }

    const data: DummyJSONResponse = await response.json();

    cachedRecipes = data.recipes.map((recipe): Recipe => ({
      id: recipe.id,
      name: recipe.name,
      ingredients: recipe.ingredients,
      instructions: recipe.instructions,
      prep_time_minutes: recipe.prepTimeMinutes,
      cook_time_minutes: recipe.cookTimeMinutes,
      total_time_minutes: recipe.prepTimeMinutes + recipe.cookTimeMinutes,
      servings: recipe.servings,
      difficulty: recipe.difficulty as 'Easy' | 'Medium' | 'Hard',
      cuisine: recipe.cuisine,
      calories_per_serving: recipe.caloriesPerServing,
      tags: recipe.tags,
      image: recipe.image,
      rating: recipe.rating,
      review_count: recipe.reviewCount,
      meal_type: recipe.mealType,
    }));

    return cachedRecipes;
  } catch (error) {
    console.error('Failed to fetch recipes:', error);
    return getFallbackRecipes();
  }
}

export async function getRecipeById(id: number): Promise<Recipe | undefined> {
  const recipes = await getRecipes();
  return recipes.find((recipe) => recipe.id === id);
}

export async function getRecipesByCuisine(cuisine: string): Promise<Recipe[]> {
  const recipes = await getRecipes();
  return recipes.filter((recipe) => recipe.cuisine === cuisine);
}

export async function getRecipesByDifficulty(difficulty: string): Promise<Recipe[]> {
  const recipes = await getRecipes();
  return recipes.filter((recipe) => recipe.difficulty === difficulty);
}

export async function getRecipesByMealType(mealType: string): Promise<Recipe[]> {
  const recipes = await getRecipes();
  return recipes.filter((recipe) =>
    recipe.meal_type.some((mt) => mt.toLowerCase() === mealType.toLowerCase())
  );
}

export async function getRecipeCuisines(): Promise<string[]> {
  const recipes = await getRecipes();
  return [...new Set(recipes.map((recipe) => recipe.cuisine))].sort();
}

export async function getRecipeDifficulties(): Promise<string[]> {
  return ['Easy', 'Medium', 'Hard'];
}

export async function getRecipeMealTypes(): Promise<string[]> {
  const recipes = await getRecipes();
  const allMealTypes = recipes.flatMap((recipe) => recipe.meal_type);
  return [...new Set(allMealTypes)].sort();
}

export async function getFeaturedRecipes(): Promise<Recipe[]> {
  const recipes = await getRecipes();
  return recipes.filter((recipe) => recipe.rating >= 4.5).slice(0, 6);
}

export async function getQuickRecipes(): Promise<Recipe[]> {
  const recipes = await getRecipes();
  return recipes
    .filter((recipe) => recipe.total_time_minutes <= 30)
    .sort((a, b) => a.total_time_minutes - b.total_time_minutes)
    .slice(0, 6);
}

function getFallbackRecipes(): Recipe[] {
  return [
    {
      id: 1,
      name: 'Classic Spaghetti Carbonara',
      ingredients: [
        '400g spaghetti',
        '200g pancetta or guanciale',
        '4 large eggs',
        '100g Pecorino Romano cheese',
        'Black pepper to taste',
        'Salt to taste',
      ],
      instructions: [
        'Bring a large pot of salted water to boil and cook spaghetti until al dente.',
        'While pasta cooks, cut pancetta into small cubes and fry until crispy.',
        'In a bowl, whisk eggs with grated cheese and plenty of black pepper.',
        'Drain pasta, reserving some cooking water.',
        'Toss hot pasta with pancetta, then remove from heat.',
        'Add egg mixture and toss quickly, adding pasta water if needed.',
        'Serve immediately with extra cheese and pepper.',
      ],
      prep_time_minutes: 10,
      cook_time_minutes: 20,
      total_time_minutes: 30,
      servings: 4,
      difficulty: 'Medium',
      cuisine: 'Italian',
      calories_per_serving: 550,
      tags: ['pasta', 'italian', 'quick', 'dinner'],
      image: 'https://cdn.dummyjson.com/recipe-images/1.webp',
      rating: 4.8,
      review_count: 256,
      meal_type: ['Dinner', 'Lunch'],
    },
    {
      id: 2,
      name: 'Simple Avocado Toast',
      ingredients: [
        '2 slices sourdough bread',
        '1 ripe avocado',
        'Salt and pepper',
        'Red pepper flakes',
        'Lemon juice',
      ],
      instructions: [
        'Toast the bread until golden and crispy.',
        'Cut avocado in half, remove pit, and scoop into a bowl.',
        'Mash with a fork and season with salt, pepper, and lemon.',
        'Spread onto toast and top with red pepper flakes.',
      ],
      prep_time_minutes: 5,
      cook_time_minutes: 5,
      total_time_minutes: 10,
      servings: 1,
      difficulty: 'Easy',
      cuisine: 'American',
      calories_per_serving: 320,
      tags: ['breakfast', 'quick', 'healthy', 'vegetarian'],
      image: 'https://cdn.dummyjson.com/recipe-images/2.webp',
      rating: 4.5,
      review_count: 189,
      meal_type: ['Breakfast', 'Snack'],
    },
  ];
}

export function formatTime(minutes: number): string {
  if (minutes < 60) {
    return `${minutes} min`;
  }
  const hours = Math.floor(minutes / 60);
  const mins = minutes % 60;
  if (mins === 0) {
    return `${hours} hr`;
  }
  return `${hours} hr ${mins} min`;
}

export function formatRating(rating: number): string {
  return rating.toFixed(1);
}

export function getDifficultyColor(difficulty: string): string {
  switch (difficulty) {
    case 'Easy':
      return 'badge-green';
    case 'Medium':
      return 'badge-yellow';
    case 'Hard':
      return 'badge-red';
    default:
      return 'badge-gray';
  }
}

export function getCalorieLabel(calories: number): { label: string; color: string } {
  if (calories < 300) {
    return { label: 'Light', color: 'badge-green' };
  } else if (calories < 500) {
    return { label: 'Moderate', color: 'badge-yellow' };
  } else {
    return { label: 'Hearty', color: 'badge-red' };
  }
}
