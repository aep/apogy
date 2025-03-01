#!/usr/bin/env bun
import { DefaultService, Document, OpenAPI } from '../api/ts/client';
import { randomUUID } from 'crypto';

// Configure the OpenAPI client
OpenAPI.BASE = 'http://localhost:27666';

// List of possible author first names and last names for random generation
const firstNames = [
	'Alice', 'Bob', 'Charlie', 'David', 'Emma', 'Frank', 'Grace', 'Henry',
	'Isabella', 'Jack', 'Katherine', 'Liam', 'Mia', 'Noah', 'Olivia', 'Peter',
	'Quinn', 'Rachel', 'Samuel', 'Tara', 'Uma', 'Victor', 'Wendy', 'Xavier',
	'Yara', 'Zack'
];

const lastNames = [
	'Smith', 'Johnson', 'Williams', 'Brown', 'Jones', 'Garcia', 'Miller',
	'Davis', 'Rodriguez', 'Martinez', 'Hernandez', 'Lopez', 'Gonzalez',
	'Wilson', 'Anderson', 'Thomas', 'Taylor', 'Moore', 'Jackson', 'Martin',
	'Lee', 'Perez', 'Thompson', 'White', 'Harris', 'Sanchez'
];

// List of possible book title parts for random generation
const titlePrefixes = [
	'The', 'A', 'Tales of', 'Journey to', 'Chronicles of', 'Secrets of',
	'Mystery of', 'Adventures in', 'Beyond the', 'Inside', 'Through the'
];

const titleWords = [
	'Dragon', 'Star', 'Moon', 'Sun', 'Mountain', 'Ocean', 'Forest', 'City',
	'Dream', 'Shadow', 'Light', 'Dark', 'Time', 'Space', 'World', 'Heart',
	'Mind', 'Soul', 'Spirit', 'Garden', 'Desert', 'River', 'Sky', 'Storm',
	'Winter', 'Summer', 'Spring', 'Autumn'
];

const titleSuffixes = [
	'Quest', 'Chronicles', 'Tales', 'Story', 'Journey', 'Adventure',
	'Mystery', 'Saga', 'Legend', 'Myth', 'Prophecy', 'Legacy'
];

// Function to generate a random book title
function generateTitle(): string {
	const usePrefix = Math.random() > 0.3;
	const useSuffix = Math.random() > 0.5;

	const parts: string[] = [];
	if (usePrefix) {
		parts.push(titlePrefixes[Math.floor(Math.random() * titlePrefixes.length)]);
	}
	parts.push(titleWords[Math.floor(Math.random() * titleWords.length)]);
	if (useSuffix) {
		parts.push(titleSuffixes[Math.floor(Math.random() * titleSuffixes.length)]);
	}

	return parts.join(' ');
}

// Function to generate a random author name
function generateAuthor(): string {
	const firstName = firstNames[Math.floor(Math.random() * firstNames.length)];
	const lastName = lastNames[Math.floor(Math.random() * lastNames.length)];
	return `${firstName} ${lastName}`;
}

// Function to generate a random ISBN
function generateISBN(): string {
	// Generate a 13-digit ISBN
	const isbn = Array.from({ length: 13 }, () => Math.floor(Math.random() * 10)).join('');
	return isbn;
}

// Function to generate a random book
function generateBook(): Document {
	return {
		id: randomUUID(),
		model: 'com.example.Book',
		val: {
			name: generateTitle(),
			author: generateAuthor(),
			isbn: generateISBN(),
			reviews: [] // Start with no reviews
		}
	};
}

// Main function to generate and store books
async function generateMillionBooks() {
	console.log('Starting to generate 1 million books...');
	const batchSize = 1000; // Process in batches to manage memory
	const totalBooks = 1_000_000;
	const totalBatches = totalBooks / batchSize;

	for (let batch = 0; batch < totalBatches; batch++) {
		const promises: Promise<any>[] = [];

		for (let i = 0; i < batchSize; i++) {
			const book = generateBook();
			promises.push(DefaultService.putDocument(book));
		}

		try {
			await Promise.all(promises);
			console.log(`Completed batch ${batch + 1}/${totalBatches}`);
		} catch (error) {
			console.error(`Error in batch ${batch + 1}:`, error);
			throw error;
		}
	}

	console.log('Successfully generated 1 million books!');
}

// Run the generator
generateMillionBooks().catch(console.error);
