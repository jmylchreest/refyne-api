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
  // USA (USD)
  { city: 'San Francisco, CA', country: 'United States', currency: 'USD', salaryMin: 120000, salaryMax: 250000 },
  { city: 'New York, NY', country: 'United States', currency: 'USD', salaryMin: 110000, salaryMax: 230000 },
  { city: 'Seattle, WA', country: 'United States', currency: 'USD', salaryMin: 115000, salaryMax: 240000 },
  { city: 'Austin, TX', country: 'United States', currency: 'USD', salaryMin: 100000, salaryMax: 200000 },
  { city: 'Boston, MA', country: 'United States', currency: 'USD', salaryMin: 105000, salaryMax: 210000 },
  { city: 'Denver, CO', country: 'United States', currency: 'USD', salaryMin: 95000, salaryMax: 190000 },
  { city: 'Chicago, IL', country: 'United States', currency: 'USD', salaryMin: 95000, salaryMax: 195000 },
  { city: 'Los Angeles, CA', country: 'United States', currency: 'USD', salaryMin: 110000, salaryMax: 220000 },
  // UK (GBP)
  { city: 'London', country: 'United Kingdom', currency: 'GBP', salaryMin: 65000, salaryMax: 150000 },
  { city: 'Manchester', country: 'United Kingdom', currency: 'GBP', salaryMin: 50000, salaryMax: 110000 },
  { city: 'Edinburgh', country: 'United Kingdom', currency: 'GBP', salaryMin: 50000, salaryMax: 105000 },
  { city: 'Bristol', country: 'United Kingdom', currency: 'GBP', salaryMin: 48000, salaryMax: 100000 },
  { city: 'Cambridge', country: 'United Kingdom', currency: 'GBP', salaryMin: 55000, salaryMax: 120000 },
  // Europe (EUR)
  { city: 'Berlin', country: 'Germany', currency: 'EUR', salaryMin: 60000, salaryMax: 120000 },
  { city: 'Munich', country: 'Germany', currency: 'EUR', salaryMin: 65000, salaryMax: 130000 },
  { city: 'Amsterdam', country: 'Netherlands', currency: 'EUR', salaryMin: 60000, salaryMax: 125000 },
  { city: 'Paris', country: 'France', currency: 'EUR', salaryMin: 55000, salaryMax: 115000 },
  { city: 'Dublin', country: 'Ireland', currency: 'EUR', salaryMin: 65000, salaryMax: 140000 },
  { city: 'Stockholm', country: 'Sweden', currency: 'EUR', salaryMin: 55000, salaryMax: 110000 },
  { city: 'Barcelona', country: 'Spain', currency: 'EUR', salaryMin: 45000, salaryMax: 90000 },
  { city: 'Lisbon', country: 'Portugal', currency: 'EUR', salaryMin: 40000, salaryMax: 85000 },
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

function generateJob(index: number): Job {
  const title = JOB_TITLES[index % JOB_TITLES.length];
  const locationData = faker.helpers.arrayElement(LOCATIONS);
  const workMode = faker.helpers.arrayElement(WORK_MODES);

  // Generate salary within the location's range
  const salaryBase = faker.number.int({
    min: locationData.salaryMin,
    max: locationData.salaryMax - 20000
  });
  const salaryRange = faker.number.int({ min: 15000, max: 40000 });

  // For remote jobs, use a mixed approach - could be based anywhere
  const isFullyRemote = workMode === 'remote';
  const displayLocation = isFullyRemote
    ? `Remote (${locationData.country})`
    : workMode === 'hybrid'
      ? `${locationData.city} (Hybrid)`
      : locationData.city;

  const companyName = faker.company.name();

  return {
    id: `job-${index + 1}`,
    title,
    company: companyName,
    location: displayLocation,
    city: locationData.city,
    country: locationData.country,
    work_mode: workMode,
    salary_min: salaryBase,
    salary_max: salaryBase + salaryRange,
    currency: locationData.currency,
    type: faker.helpers.arrayElement(JOB_TYPES),
    category: faker.helpers.arrayElement(CATEGORIES),
    description: faker.lorem.paragraphs(3),
    requirements: faker.helpers.arrayElements(REQUIREMENTS, { min: 4, max: 7 }),
    benefits: faker.helpers.arrayElements(BENEFITS, { min: 5, max: 10 }),
    posted_at: faker.date.recent({ days: 30 }),
    company_logo: `https://api.dicebear.com/7.x/initials/svg?seed=${encodeURIComponent(companyName)}&backgroundColor=6366f1`,
  };
}

// Generate 75 jobs
const allJobs: Job[] = Array.from({ length: 75 }, (_, i) => generateJob(i));

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
