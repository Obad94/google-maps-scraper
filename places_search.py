#!/usr/bin/env python3
"""
Google Places API Nearby Search Script
Searches for nearby places based on location, radius, and keyword/category
Results are saved to CSV with cost calculation
"""

import argparse
import csv
import json
import os
import sys
from datetime import datetime
from typing import Dict, List, Optional

import requests
from dotenv import load_dotenv


# Google Places API pricing (as of 2024)
# Nearby Search (New) costs are based on fields requested
FIELD_COSTS = {
    # Basic fields ($0.017 per request for the call + field cost)
    "id": 0.0,
    "displayName": 0.0,
    "formattedAddress": 0.0,
    "location": 0.0,
    "types": 0.0,
    "primaryType": 0.0,
    "primaryTypeDisplayName": 0.0,
    "shortFormattedAddress": 0.0,
    "googleMapsUri": 0.0,

    # Advanced fields (additional $0.005 per field)
    "nationalPhoneNumber": 0.005,
    "internationalPhoneNumber": 0.005,
    "rating": 0.005,
    "userRatingCount": 0.005,
    "businessStatus": 0.005,
    "priceLevel": 0.005,
    "websiteUri": 0.005,
    "regularOpeningHours": 0.005,
    "utcOffsetMinutes": 0.005,

    # Preferred fields (additional $0.007 per field)
    "currentOpeningHours": 0.007,
    "currentSecondaryOpeningHours": 0.007,
    "regularSecondaryOpeningHours": 0.007,
    "editorialSummary": 0.007,
    "reviews": 0.007,
    "photos": 0.007,
}

BASE_NEARBY_SEARCH_COST = 0.017  # $0.017 per Nearby Search request


class PlacesSearcher:
    """Handles Google Places API nearby search operations"""

    def __init__(self, api_key: str, data_level: str = "id_only", debug: bool = False):
        """
        Initialize the Places searcher

        Args:
            api_key: Google Maps API key
            data_level: Level of data to fetch ("id_only", "basic", "advanced")
            debug: Enable debug output
        """
        self.api_key = api_key
        self.data_level = data_level
        self.debug = debug
        self.base_url = "https://places.googleapis.com/v1/places:searchNearby"
        self.headers = {
            "Content-Type": "application/json",
            "X-Goog-Api-Key": api_key,
            "X-Goog-FieldMask": self._get_field_mask()
        }

    def _get_field_mask(self) -> str:
        """
        Get the field mask for the API request

        Returns:
            Comma-separated list of fields to request
        """
        if self.data_level == "id_only":
            # Minimum fields (ID + displayName for valid response)
            # displayName is Basic field (no extra cost)
            fields = ["places.id", "places.displayName"]
        elif self.data_level == "basic":
            # All Basic fields (no extra cost beyond base $0.017)
            fields = [
                "places.id",
                "places.displayName",
                "places.formattedAddress",
                "places.location",
                "places.types",
                "places.primaryType",
                "places.googleMapsUri"
            ]
        elif self.data_level == "advanced":
            # Basic + Advanced fields (adds $0.005 per field)
            fields = [
                "places.id",
                "places.displayName",
                "places.formattedAddress",
                "places.location",
                "places.types",
                "places.primaryType",
                "places.googleMapsUri",
                "places.rating",
                "places.userRatingCount",
                "places.nationalPhoneNumber",
                "places.internationalPhoneNumber",
                "places.websiteUri",
                "places.businessStatus"
            ]
        else:
            fields = ["places.id", "places.displayName"]

        return ",".join(fields)

    def calculate_request_cost(self) -> float:
        """
        Calculate the cost of a single API request based on fields requested

        Returns:
            Cost in USD
        """
        cost = BASE_NEARBY_SEARCH_COST

        # Parse fields from the mask
        field_mask = self.headers["X-Goog-FieldMask"]
        fields = [f.replace("places.", "") for f in field_mask.split(",")]

        for field in fields:
            if field in FIELD_COSTS:
                cost += FIELD_COSTS[field]

        return cost

    def search_nearby(
        self,
        latitude: float,
        longitude: float,
        radius: float,
        keyword: Optional[str] = None,
        include_types: Optional[List[str]] = None
    ) -> tuple[List[Dict], int]:
        """
        Search for nearby places with pagination support

        Args:
            latitude: Latitude coordinate
            longitude: Longitude coordinate
            radius: Search radius in meters
            keyword: Keyword to search for (optional)
            include_types: List of place types to include (optional)

        Returns:
            Tuple of (list of all places, number of requests made)
        """
        all_places = []
        next_page_token = None
        request_count = 0

        while True:
            payload = {
                "locationRestriction": {
                    "circle": {
                        "center": {
                            "latitude": latitude,
                            "longitude": longitude
                        },
                        "radius": radius
                    }
                },
                "maxResultCount": 20  # Maximum allowed by API
            }

            # Add included types if provided
            if include_types:
                payload["includedTypes"] = include_types

            # Add rank preference
            payload["rankPreference"] = "DISTANCE"

            # Add page token for pagination
            if next_page_token:
                payload["pageToken"] = next_page_token

            try:
                # Debug: Print request details
                if self.debug:
                    print(f"\n--- API Request ---")
                    print(f"URL: {self.base_url}")
                    print(f"Headers: {json.dumps(self.headers, indent=2)}")
                    print(f"Payload: {json.dumps(payload, indent=2)}")
                    print("--- End Request ---\n")

                response = requests.post(
                    self.base_url,
                    headers=self.headers,
                    json=payload,
                    timeout=30
                )

                # Debug: Print raw response
                if self.debug:
                    print(f"\n--- Raw Response ---")
                    print(f"Status Code: {response.status_code}")
                    print(f"Response Headers: {dict(response.headers)}")
                    print(f"Response Text: {response.text}")
                    print("--- End Raw Response ---\n")

                response.raise_for_status()
                result = response.json()
                request_count += 1

                # Debug output
                if self.debug:
                    print(f"\n--- Parsed API Response (Page {request_count}) ---")
                    print(json.dumps(result, indent=2))
                    print("--- End Response ---\n")

                # Add places from this page
                places = result.get("places", [])
                all_places.extend(places)

                print(f"Fetched page {request_count}: {len(places)} places (Total so far: {len(all_places)})")

                # Check for next page
                next_page_token = result.get("nextPageToken")

                # Debug: Show if token is present
                if next_page_token:
                    print(f"  → Next page token found, fetching more results...")
                else:
                    print(f"  → No more pages available (no nextPageToken in response)")

                if not next_page_token:
                    break

            except requests.exceptions.RequestException as e:
                print(f"Error making API request: {e}")
                if hasattr(e.response, 'text'):
                    print(f"Response: {e.response.text}")
                sys.exit(1)

        return all_places, request_count

    def save_to_csv(self, places: List[Dict], output_file: str):
        """
        Save search results to CSV file

        Args:
            places: List of place dictionaries
            output_file: Output CSV file path
        """
        if not places:
            print("No places found in the results")
            return

        # Define CSV columns based on data level
        if self.data_level == "id_only":
            fieldnames = ["place_id", "google_maps_url"]
        elif self.data_level == "basic":
            fieldnames = [
                "place_id",
                "name",
                "address",
                "latitude",
                "longitude",
                "types",
                "primary_type",
                "google_maps_url"
            ]
        elif self.data_level == "advanced":
            fieldnames = [
                "place_id",
                "name",
                "address",
                "latitude",
                "longitude",
                "types",
                "primary_type",
                "google_maps_url",
                "rating",
                "user_ratings_total",
                "phone_national",
                "phone_international",
                "website",
                "business_status"
            ]
        else:
            fieldnames = ["place_id"]

        with open(output_file, 'w', newline='', encoding='utf-8') as csvfile:
            writer = csv.DictWriter(csvfile, fieldnames=fieldnames)
            writer.writeheader()

            for place in places:
                if self.data_level == "id_only":
                    place_id = place.get("id", "")
                    row = {
                        "place_id": place_id,
                        "google_maps_url": f"https://www.google.com/maps/place/?q=place_id:{place_id}"
                    }
                elif self.data_level == "basic":
                    row = {
                        "place_id": place.get("id", ""),
                        "name": place.get("displayName", {}).get("text", ""),
                        "address": place.get("formattedAddress", ""),
                        "latitude": place.get("location", {}).get("latitude", ""),
                        "longitude": place.get("location", {}).get("longitude", ""),
                        "types": ", ".join(place.get("types", [])),
                        "primary_type": place.get("primaryType", ""),
                        "google_maps_url": place.get("googleMapsUri", "")
                    }
                elif self.data_level == "advanced":
                    row = {
                        "place_id": place.get("id", ""),
                        "name": place.get("displayName", {}).get("text", ""),
                        "address": place.get("formattedAddress", ""),
                        "latitude": place.get("location", {}).get("latitude", ""),
                        "longitude": place.get("location", {}).get("longitude", ""),
                        "types": ", ".join(place.get("types", [])),
                        "primary_type": place.get("primaryType", ""),
                        "google_maps_url": place.get("googleMapsUri", ""),
                        "rating": place.get("rating", ""),
                        "user_ratings_total": place.get("userRatingCount", ""),
                        "phone_national": place.get("nationalPhoneNumber", ""),
                        "phone_international": place.get("internationalPhoneNumber", ""),
                        "website": place.get("websiteUri", ""),
                        "business_status": place.get("businessStatus", "")
                    }
                writer.writerow(row)

        print(f"\nResults saved to: {output_file}")
        print(f"Total places found: {len(places)}")


def main():
    """Main function to run the script"""
    parser = argparse.ArgumentParser(
        description="Search for nearby places using Google Places API",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Get only place IDs (default, cheapest - $0.017 per request)
  python places_search.py --lat 40.7128 --lng -74.0060 --radius 500 --category restaurant

  # Get basic data (name, address, location, types - same cost $0.017)
  python places_search.py --lat 40.7128 --lng -74.0060 --radius 500 --category cafe --data-level basic

  # Get advanced data (includes rating, phone, website - $0.047 per request)
  python places_search.py --lat 40.7128 --lng -74.0060 --radius 500 --category hotel --data-level advanced

  # Search with multiple categories
  python places_search.py --lat 40.7128 --lng -74.0060 --radius 1000 --category restaurant,cafe,bar --data-level basic

  # Custom output file
  python places_search.py --lat 40.7128 --lng -74.0060 --radius 500 --category restaurant --output my_results.csv

Data Levels:
  id_only   - Only place IDs (cheapest, $0.017/request)
  basic     - ID, name, address, location, types, maps URL ($0.017/request)
  advanced  - Basic + rating, reviews, phone, website, status ($0.047/request)

Common place types:
  restaurant, cafe, bar, bakery, hotel, gas_station, parking, pharmacy,
  hospital, bank, atm, supermarket, grocery_or_supermarket, store, gym,
  museum, park, library, school, airport, etc.
        """
    )

    parser.add_argument(
        '--lat',
        type=float,
        required=True,
        help='Latitude coordinate (e.g., 40.7128)'
    )

    parser.add_argument(
        '--lng',
        type=float,
        required=True,
        help='Longitude coordinate (e.g., -74.0060)'
    )

    parser.add_argument(
        '--radius',
        type=float,
        required=True,
        help='Search radius in meters (e.g., 500)'
    )

    parser.add_argument(
        '--category',
        type=str,
        help='Category/type of places to search (e.g., restaurant,cafe,bar). Comma-separated for multiple.'
    )

    parser.add_argument(
        '--output',
        type=str,
        default=None,
        help='Output CSV file path (default: places_TIMESTAMP.csv)'
    )

    parser.add_argument(
        '--data-level',
        type=str,
        choices=['id_only', 'basic', 'advanced'],
        default='id_only',
        help='Data level to fetch: id_only (default, cheapest), basic (no extra cost), advanced (adds cost)'
    )

    parser.add_argument(
        '--debug',
        action='store_true',
        help='Enable debug output to see API responses'
    )

    args = parser.parse_args()

    # Load environment variables
    load_dotenv()
    api_key = os.getenv('GOOGLE_MAPS_API_KEY')

    if not api_key:
        print("Error: GOOGLE_MAPS_API_KEY not found in .env file")
        sys.exit(1)

    # Parse categories/types
    include_types = None
    if args.category:
        include_types = [t.strip() for t in args.category.split(',')]

    # Generate output filename if not provided
    if args.output is None:
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        args.output = f"places_{timestamp}.csv"

    # Initialize searcher
    searcher = PlacesSearcher(api_key, data_level=args.data_level, debug=args.debug)

    # Calculate and display cost
    cost_per_request = searcher.calculate_request_cost()
    print(f"\n{'='*60}")
    print(f"Google Places API - Nearby Search")
    print(f"{'='*60}")
    print(f"Location: ({args.lat}, {args.lng})")
    print(f"Radius: {args.radius} meters")
    print(f"Category: {args.category if args.category else 'All types'}")
    print(f"Data Level: {args.data_level}")
    print(f"\nEstimated cost per request: ${cost_per_request:.4f} USD")
    print(f"(Based on Google's official pricing documentation)")
    print(f"Note: Actual costs are tracked in your Google Cloud Console")
    print(f"{'='*60}\n")

    # Perform search
    print("Searching for nearby places (fetching all pages)...\n")
    all_places, request_count = searcher.search_nearby(
        latitude=args.lat,
        longitude=args.lng,
        radius=args.radius,
        include_types=include_types
    )

    # Save results
    searcher.save_to_csv(all_places, args.output)

    # Calculate total cost
    total_cost = cost_per_request * request_count

    # Display summary
    print(f"\n{'='*60}")
    print(f"Search completed successfully!")
    print(f"Total pages fetched: {request_count}")
    print(f"Total places found: {len(all_places)}")
    print(f"Estimated cost per request: ${cost_per_request:.4f} USD")
    print(f"Estimated total cost: ${total_cost:.4f} USD")
    print(f"\nView actual costs at: https://console.cloud.google.com/billing")
    print(f"{'='*60}\n")


if __name__ == "__main__":
    main()
