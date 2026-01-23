import { faker } from '@faker-js/faker';

export type WorkMode = 'remote' | 'hybrid' | 'on-site';
export type Currency = 'USD' | 'EUR' | 'GBP';

export interface Job {
  id: string;
  title: string;
  company: string;
  location: string;
  city: string;
  country: string;
  work_mode: WorkMode;
  salary_min: number;
  salary_max: number;
  currency: Currency;
  type: 'full-time' | 'part-time' | 'contract' | 'freelance';
  category: string;
  description: string;
  requirements: string[];
  benefits: string[];
  posted_at: Date;
  company_logo: string;
}

const JOB_TITLES = [
  'Senior Software Engineer',
  'Frontend Developer',
  'Backend Developer',
  'Full Stack Developer',
  'DevOps Engineer',
  'Data Scientist',
  'Product Manager',
  'UX Designer',
  'QA Engineer',
  'Machine Learning Engineer',
  'Cloud Architect',
  'Security Engineer',
  'Mobile Developer',
  'Technical Lead',
  'Engineering Manager',
  'Site Reliability Engineer',
  'Data Engineer',
  'Solutions Architect',
  'Platform Engineer',
  'Infrastructure Engineer',
  // Entry-level and lower-paying positions
  'Junior Software Developer',
  'Junior Frontend Developer',
  'Junior QA Tester',
  'Software Development Intern',
  'Technical Support Specialist',
  'IT Help Desk Technician',
  'Data Entry Clerk',
  'Junior Data Analyst',
  'Web Content Coordinator',
  'IT Support Intern',
  'Graduate Software Engineer',
  'Trainee Developer',
  'Associate QA Analyst',
  'Junior Systems Administrator',
  'Entry Level IT Technician',
];

const CATEGORIES = [
  'Engineering',
  'Design',
  'Product',
  'Data Science',
  'DevOps',
  'Security',
  'Management',
];

// Location data with currency and salary ranges appropriate for each region
interface LocationData {
  city: string;
  country: string;
  currency: Currency;
  salaryMin: number;
  salaryMax: number;
}

const LOCATIONS: LocationData[] = [
  // USA - High cost of living (USD)
  { city: 'San Francisco, CA', country: 'United States', currency: 'USD', salaryMin: 120000, salaryMax: 250000 },
  { city: 'New York, NY', country: 'United States', currency: 'USD', salaryMin: 110000, salaryMax: 230000 },
  { city: 'Seattle, WA', country: 'United States', currency: 'USD', salaryMin: 115000, salaryMax: 240000 },
  { city: 'Los Angeles, CA', country: 'United States', currency: 'USD', salaryMin: 110000, salaryMax: 220000 },
  { city: 'Boston, MA', country: 'United States', currency: 'USD', salaryMin: 105000, salaryMax: 210000 },
  // USA - Medium cost of living (USD)
  { city: 'Austin, TX', country: 'United States', currency: 'USD', salaryMin: 85000, salaryMax: 180000 },
  { city: 'Denver, CO', country: 'United States', currency: 'USD', salaryMin: 80000, salaryMax: 170000 },
  { city: 'Chicago, IL', country: 'United States', currency: 'USD', salaryMin: 80000, salaryMax: 175000 },
  { city: 'Atlanta, GA', country: 'United States', currency: 'USD', salaryMin: 75000, salaryMax: 160000 },
  { city: 'Phoenix, AZ', country: 'United States', currency: 'USD', salaryMin: 70000, salaryMax: 150000 },
  // USA - Lower cost of living (USD)
  { city: 'Indianapolis, IN', country: 'United States', currency: 'USD', salaryMin: 55000, salaryMax: 120000 },
  { city: 'Columbus, OH', country: 'United States', currency: 'USD', salaryMin: 55000, salaryMax: 115000 },
  { city: 'Kansas City, MO', country: 'United States', currency: 'USD', salaryMin: 50000, salaryMax: 110000 },
  { city: 'Louisville, KY', country: 'United States', currency: 'USD', salaryMin: 45000, salaryMax: 100000 },
  { city: 'Oklahoma City, OK', country: 'United States', currency: 'USD', salaryMin: 42000, salaryMax: 95000 },
  // UK (GBP)
  { city: 'London', country: 'United Kingdom', currency: 'GBP', salaryMin: 65000, salaryMax: 150000 },
  { city: 'Manchester', country: 'United Kingdom', currency: 'GBP', salaryMin: 40000, salaryMax: 90000 },
  { city: 'Edinburgh', country: 'United Kingdom', currency: 'GBP', salaryMin: 38000, salaryMax: 85000 },
  { city: 'Bristol', country: 'United Kingdom', currency: 'GBP', salaryMin: 35000, salaryMax: 80000 },
  { city: 'Cambridge', country: 'United Kingdom', currency: 'GBP', salaryMin: 45000, salaryMax: 100000 },
  { city: 'Leeds', country: 'United Kingdom', currency: 'GBP', salaryMin: 30000, salaryMax: 70000 },
  { city: 'Birmingham', country: 'United Kingdom', currency: 'GBP', salaryMin: 28000, salaryMax: 65000 },
  { city: 'Newcastle', country: 'United Kingdom', currency: 'GBP', salaryMin: 26000, salaryMax: 60000 },
  // Europe - Higher paying (EUR)
  { city: 'Berlin', country: 'Germany', currency: 'EUR', salaryMin: 55000, salaryMax: 110000 },
  { city: 'Munich', country: 'Germany', currency: 'EUR', salaryMin: 60000, salaryMax: 120000 },
  { city: 'Amsterdam', country: 'Netherlands', currency: 'EUR', salaryMin: 55000, salaryMax: 115000 },
  { city: 'Dublin', country: 'Ireland', currency: 'EUR', salaryMin: 55000, salaryMax: 120000 },
  // Europe - Medium (EUR)
  { city: 'Paris', country: 'France', currency: 'EUR', salaryMin: 45000, salaryMax: 95000 },
  { city: 'Stockholm', country: 'Sweden', currency: 'EUR', salaryMin: 45000, salaryMax: 95000 },
  { city: 'Vienna', country: 'Austria', currency: 'EUR', salaryMin: 42000, salaryMax: 90000 },
  // Europe - Lower cost of living (EUR)
  { city: 'Barcelona', country: 'Spain', currency: 'EUR', salaryMin: 32000, salaryMax: 70000 },
  { city: 'Madrid', country: 'Spain', currency: 'EUR', salaryMin: 30000, salaryMax: 65000 },
  { city: 'Lisbon', country: 'Portugal', currency: 'EUR', salaryMin: 25000, salaryMax: 55000 },
  { city: 'Prague', country: 'Czech Republic', currency: 'EUR', salaryMin: 28000, salaryMax: 60000 },
  { city: 'Warsaw', country: 'Poland', currency: 'EUR', salaryMin: 25000, salaryMax: 55000 },
  { city: 'Budapest', country: 'Hungary', currency: 'EUR', salaryMin: 22000, salaryMax: 50000 },
  { city: 'Bucharest', country: 'Romania', currency: 'EUR', salaryMin: 20000, salaryMax: 45000 },
  { city: 'Sofia', country: 'Bulgaria', currency: 'EUR', salaryMin: 18000, salaryMax: 40000 },
];

const WORK_MODES: WorkMode[] = ['remote', 'hybrid', 'on-site'];

const JOB_TYPES: Job['type'][] = ['full-time', 'part-time', 'contract', 'freelance'];

const REQUIREMENTS = [
  '5+ years of experience in software development',
  'Strong proficiency in TypeScript and JavaScript',
  'Experience with modern frontend frameworks (React, Vue, or Angular)',
  'Experience with cloud platforms (AWS, GCP, or Azure)',
  'Strong problem-solving and debugging skills',
  'Excellent communication and collaboration abilities',
  'Experience with CI/CD pipelines',
  'Knowledge of containerization (Docker, Kubernetes)',
  'Experience with database systems (SQL and NoSQL)',
  'Understanding of microservices architecture',
  'Experience with agile methodologies',
  'Strong attention to detail',
  'Experience with RESTful APIs and GraphQL',
  'Knowledge of security best practices',
  'Experience mentoring junior developers',
];

const BENEFITS = [
  'Competitive salary and equity',
  'Health, dental, and vision insurance',
  '401(k) with company match',
  'Unlimited PTO',
  'Remote work flexibility',
  'Learning and development budget',
  'Home office stipend',
  'Gym membership',
  'Mental health support',
  'Parental leave',
  'Annual company retreats',
  'Stock options',
  'Flexible working hours',
  'Free lunch and snacks',
  'Career growth opportunities',
];

// Use a fixed seed for consistent data across builds
faker.seed(42);

// Determine salary modifier based on job title seniority
function getSalaryModifier(title: string): number {
  const lowerTitle = title.toLowerCase();

  // Intern positions - very low pay
  if (lowerTitle.includes('intern')) return 0.25;

  // Entry-level positions
  if (lowerTitle.includes('junior') || lowerTitle.includes('trainee') ||
      lowerTitle.includes('graduate') || lowerTitle.includes('entry level') ||
      lowerTitle.includes('associate')) return 0.45;

  // Support and clerical roles
  if (lowerTitle.includes('support') || lowerTitle.includes('help desk') ||
      lowerTitle.includes('clerk') || lowerTitle.includes('technician') ||
      lowerTitle.includes('coordinator')) return 0.35;

  // Senior/Lead positions
  if (lowerTitle.includes('senior') || lowerTitle.includes('lead') ||
      lowerTitle.includes('principal')) return 1.1;

  // Management positions
  if (lowerTitle.includes('manager') || lowerTitle.includes('director') ||
      lowerTitle.includes('architect')) return 1.25;

  // Standard mid-level
  return 0.75;
}

function generateJob(index: number): Job {
  const title = JOB_TITLES[index % JOB_TITLES.length];
  const locationData = faker.helpers.arrayElement(LOCATIONS);
  const workMode = faker.helpers.arrayElement(WORK_MODES);

  // Determine job type - internships are more likely for intern titles
  const isIntern = title.toLowerCase().includes('intern');
  const jobType = isIntern
    ? 'contract' as const
    : faker.helpers.arrayElement(JOB_TYPES);

  // Apply salary modifier based on seniority level
  const salaryModifier = getSalaryModifier(title);
  const adjustedMin = Math.round(locationData.salaryMin * salaryModifier);
  const adjustedMax = Math.round(locationData.salaryMax * salaryModifier);

  // Generate salary within the adjusted range
  const salaryBase = faker.number.int({
    min: adjustedMin,
    max: Math.max(adjustedMin, adjustedMax - 10000)
  });
  const salaryRange = faker.number.int({
    min: Math.round(5000 * salaryModifier),
    max: Math.round(25000 * salaryModifier)
  });

  // Part-time jobs get further reduced salaries (pro-rata)
  const partTimeModifier = jobType === 'part-time' ? 0.5 : 1;
  const finalSalaryMin = Math.round(salaryBase * partTimeModifier);
  const finalSalaryMax = Math.round((salaryBase + salaryRange) * partTimeModifier);

  // For remote jobs, use a mixed approach - could be based anywhere
  const isFullyRemote = workMode === 'remote';
  const displayLocation = isFullyRemote
    ? `Remote (${locationData.country})`
    : workMode === 'hybrid'
      ? `${locationData.city} (Hybrid)`
      : locationData.city;

  const companyName = faker.company.name();

  // Adjust requirements based on seniority
  const numRequirements = isIntern ? { min: 2, max: 4 } :
    salaryModifier < 0.5 ? { min: 3, max: 5 } : { min: 4, max: 7 };

  // Adjust benefits - entry level may have fewer
  const numBenefits = salaryModifier < 0.5 ? { min: 3, max: 6 } : { min: 5, max: 10 };

  return {
    id: `job-${index + 1}`,
    title,
    company: companyName,
    location: displayLocation,
    city: locationData.city,
    country: locationData.country,
    work_mode: workMode,
    salary_min: finalSalaryMin,
    salary_max: finalSalaryMax,
    currency: locationData.currency,
    type: jobType,
    category: faker.helpers.arrayElement(CATEGORIES),
    description: faker.lorem.paragraphs(3),
    requirements: faker.helpers.arrayElements(REQUIREMENTS, numRequirements),
    benefits: faker.helpers.arrayElements(BENEFITS, numBenefits),
    posted_at: faker.date.recent({ days: 30 }),
    company_logo: `https://api.dicebear.com/7.x/initials/svg?seed=${encodeURIComponent(companyName)}&backgroundColor=6366f1`,
  };
}

// Generate 100 jobs to cover all job title variations
const allJobs: Job[] = Array.from({ length: 100 }, (_, i) => generateJob(i));

export function getJobs(): Job[] {
  return allJobs;
}

export function getJobById(id: string): Job | undefined {
  return allJobs.find((job) => job.id === id);
}

export function getJobsByCategory(category: string): Job[] {
  return allJobs.filter((job) => job.category === category);
}

export function getJobsByType(type: Job['type']): Job[] {
  return allJobs.filter((job) => job.type === type);
}

export function getJobCategories(): string[] {
  return CATEGORIES;
}

export function getJobTypes(): Job['type'][] {
  return JOB_TYPES;
}

export function getJobLocations(): string[] {
  return [...new Set(allJobs.map((job) => job.location))];
}

export function getJobCountries(): string[] {
  return [...new Set(allJobs.map((job) => job.country))].sort();
}

export function getJobWorkModes(): WorkMode[] {
  return WORK_MODES;
}

export function getJobsByCountry(country: string): Job[] {
  return allJobs.filter((job) => job.country === country);
}

export function getJobsByWorkMode(workMode: WorkMode): Job[] {
  return allJobs.filter((job) => job.work_mode === workMode);
}

export function getJobsByCurrency(currency: Currency): Job[] {
  return allJobs.filter((job) => job.currency === currency);
}

export function formatSalary(min: number, max: number, currency: Currency = 'USD'): string {
  const symbols: Record<Currency, string> = {
    USD: '$',
    EUR: '\u20AC',
    GBP: '\u00A3',
  };
  const symbol = symbols[currency];
  const format = (n: number) => `${symbol}${(n / 1000).toFixed(0)}k`;
  return `${format(min)} - ${format(max)}`;
}

export function formatJobType(type: Job['type']): string {
  return type.split('-').map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(' ');
}

export function formatWorkMode(workMode: WorkMode): string {
  switch (workMode) {
    case 'remote':
      return 'Remote';
    case 'hybrid':
      return 'Hybrid';
    case 'on-site':
      return 'On-site';
    default:
      return workMode;
  }
}
