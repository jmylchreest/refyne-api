import { faker } from '@faker-js/faker';

export interface Job {
  id: string;
  title: string;
  company: string;
  location: string;
  salary_min: number;
  salary_max: number;
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

const LOCATIONS = [
  'San Francisco, CA',
  'New York, NY',
  'Seattle, WA',
  'Austin, TX',
  'Boston, MA',
  'Denver, CO',
  'Chicago, IL',
  'Los Angeles, CA',
  'Remote',
  'London, UK',
  'Berlin, Germany',
  'Toronto, Canada',
];

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
  const salaryBase = faker.number.int({ min: 80000, max: 200000 });

  return {
    id: `job-${index + 1}`,
    title,
    company: faker.company.name(),
    location: faker.helpers.arrayElement(LOCATIONS),
    salary_min: salaryBase,
    salary_max: salaryBase + faker.number.int({ min: 20000, max: 50000 }),
    type: faker.helpers.arrayElement(JOB_TYPES),
    category: faker.helpers.arrayElement(CATEGORIES),
    description: faker.lorem.paragraphs(3),
    requirements: faker.helpers.arrayElements(REQUIREMENTS, { min: 4, max: 7 }),
    benefits: faker.helpers.arrayElements(BENEFITS, { min: 5, max: 10 }),
    posted_at: faker.date.recent({ days: 30 }),
    company_logo: `https://api.dicebear.com/7.x/initials/svg?seed=${encodeURIComponent(faker.company.name())}&backgroundColor=6366f1`,
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

export function formatSalary(min: number, max: number): string {
  const format = (n: number) => `$${(n / 1000).toFixed(0)}k`;
  return `${format(min)} - ${format(max)}`;
}

export function formatJobType(type: Job['type']): string {
  return type.split('-').map((word) => word.charAt(0).toUpperCase() + word.slice(1)).join(' ');
}
