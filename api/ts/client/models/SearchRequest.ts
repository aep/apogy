/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Filter } from './Filter';
export type SearchRequest = {
    model: string;
    filters?: Array<Filter>;
    links?: Array<SearchRequest>;
    cursor?: string;
    limit?: number;
    /**
     * If true, return full documents instead of just the ids
     */
    full?: boolean;
};

