"""Configuration classes for payload generation."""

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Dict, Tuple


def _get_default_output_dir() -> str:
    """Get default output directory at the same level as payload_generator/."""
    # Get the directory where this config file is located (payload_generator/)
    config_dir = Path(__file__).parent
    # Go up one level and into payloads/
    return str(config_dir.parent / "payloads")


@dataclass
class DistConfig:
    """Controls array lengths and skew."""
    cart_items_min: int = 1
    cart_items_max: int = 10

    rec_req_ids_min: int = 1
    rec_req_ids_max: int = 20
    rec_resp_ids_min: int = 1
    rec_resp_ids_max: int = 10

    product_categories_min: int = 1
    product_categories_max: int = 40

    list_products_min: int = 10
    list_products_max: int = 50

    search_results_min: int = 0
    search_results_max: int = 20

    order_items_min: int = 1
    order_items_max: int = 10

    ad_keys_min: int = 1
    ad_keys_max: int = 60
    ads_min: int = 0
    ads_max: int = 50


@dataclass
class MoneyConfig:
    """Controls Money realism and encoding."""
    # Store units as string to avoid int64 issues in some JSON tooling (JS, etc.)
    units_as_string: bool = True
    # Default currency for *_usd fields
    default_usd_code: str = "USD"
    # Supported currencies for random selection
    currencies: Tuple[str, ...] = (
        "USD", "EUR", "JPY", "GBP", "CAD", "AUD", "CHF", "CNY", "HKD", "NZD",
        "SEK", "NOK", "DKK", "SGD", "KRW", "MXN", "INR", "BRL", "ZAR", "RUB",
        "TRY", "PLN", "THB", "IDR", "MYR", "PHP", "CZK", "HUF", "ILS", "CLP",
        "ARS", "AED", "SAR", "QAR", "KWD", "BHD", "OMR", "JOD", "EGP", "NGN",
        "KES", "TZS", "UGX", "GHS", "ETB", "MAD", "TND", "DZD", "XOF", "XAF",
        "FJD", "PGK", "VND", "KHR", "LAK", "MMK", "BDT", "LKR", "PKR",
        "NPR", "AFN", "IRR", "IQD", "LBP", "SYP", "YER", "AMD", "AZN", "GEL",
        "KZT", "UZS", "TJS", "TMT", "MNT", "TWD", "MOP", "BND"
    )
    # Units range for typical prices
    price_units_min: int = 1
    price_units_max: int = 500
    # Shipping quote range
    shipping_units_min: int = 1
    shipping_units_max: int = 500
    # Generic conversion amount range
    conversion_units_min: int = 1
    conversion_units_max: int = 2000
    # Allowed nanos options (quarter-ish plus common 0.99)
    nanos_choices: Tuple[int, ...] = (0, 250_000_000, 500_000_000, 750_000_000, 990_000_000)


@dataclass
class AddressConfig:
    countries: Tuple[str, ...] = ("US",)
    # If you want a small fixed set of realistic city/state pairs:
    city_state_pairs: Tuple[Tuple[str, str], ...] = (
        # North Carolina
        ("Durham", "NC"), ("Raleigh", "NC"), ("Charlotte", "NC"), ("Greensboro", "NC"),
        ("Winston-Salem", "NC"), ("Fayetteville", "NC"), ("Cary", "NC"), ("Wilmington", "NC"),
        # Texas
        ("Austin", "TX"), ("Houston", "TX"), ("Dallas", "TX"), ("San Antonio", "TX"),
        ("Fort Worth", "TX"), ("El Paso", "TX"), ("Arlington", "TX"), ("Corpus Christi", "TX"),
        ("Plano", "TX"), ("Laredo", "TX"), ("Lubbock", "TX"), ("Garland", "TX"),
        # Washington
        ("Seattle", "WA"), ("Spokane", "WA"), ("Tacoma", "WA"), ("Vancouver", "WA"),
        ("Bellevue", "WA"), ("Everett", "WA"), ("Kent", "WA"), ("Yakima", "WA"),
        # California
        ("San Jose", "CA"), ("Los Angeles", "CA"), ("San Francisco", "CA"), ("San Diego", "CA"),
        ("Sacramento", "CA"), ("Fresno", "CA"), ("Oakland", "CA"), ("Long Beach", "CA"),
        ("Bakersfield", "CA"), ("Anaheim", "CA"), ("Santa Ana", "CA"), ("Riverside", "CA"),
        ("Stockton", "CA"), ("Irvine", "CA"), ("Chula Vista", "CA"), ("Fremont", "CA"),
        ("San Bernardino", "CA"), ("Modesto", "CA"), ("Oxnard", "CA"), ("Fontana", "CA"),
        # New York
        ("New York", "NY"), ("Buffalo", "NY"), ("Rochester", "NY"), ("Yonkers", "NY"),
        ("Syracuse", "NY"), ("Albany", "NY"), ("New Rochelle", "NY"), ("Mount Vernon", "NY"),
        ("Schenectady", "NY"), ("Utica", "NY"), ("White Plains", "NY"), ("Hempstead", "NY"),
        # Illinois
        ("Chicago", "IL"), ("Aurora", "IL"), ("Naperville", "IL"), ("Joliet", "IL"),
        ("Rockford", "IL"), ("Elgin", "IL"), ("Peoria", "IL"), ("Champaign", "IL"),
        ("Waukegan", "IL"), ("Cicero", "IL"), ("Bloomington", "IL"), ("Arlington Heights", "IL"),
        # Massachusetts
        ("Boston", "MA"), ("Worcester", "MA"), ("Springfield", "MA"), ("Lowell", "MA"),
        ("Cambridge", "MA"), ("New Bedford", "MA"), ("Brockton", "MA"), ("Quincy", "MA"),
        ("Lynn", "MA"), ("Fall River", "MA"), ("Newton", "MA"), ("Lawrence", "MA"),
        # Florida
        ("Miami", "FL"), ("Tampa", "FL"), ("Orlando", "FL"), ("Jacksonville", "FL"),
        ("Tallahassee", "FL"), ("St. Petersburg", "FL"), ("Hialeah", "FL"), ("Port St. Lucie", "FL"),
        ("Cape Coral", "FL"), ("Fort Lauderdale", "FL"), ("Pembroke Pines", "FL"), ("Hollywood", "FL"),
        ("Miramar", "FL"), ("Gainesville", "FL"), ("Coral Springs", "FL"), ("Miami Gardens", "FL"),
        # Pennsylvania
        ("Philadelphia", "PA"), ("Pittsburgh", "PA"), ("Allentown", "PA"), ("Erie", "PA"),
        ("Reading", "PA"), ("Scranton", "PA"), ("Bethlehem", "PA"), ("Lancaster", "PA"),
        ("Harrisburg", "PA"), ("Altoona", "PA"), ("York", "PA"), ("State College", "PA"),
        # Ohio
        ("Columbus", "OH"), ("Cleveland", "OH"), ("Cincinnati", "OH"), ("Toledo", "OH"),
        ("Akron", "OH"), ("Dayton", "OH"), ("Parma", "OH"), ("Canton", "OH"),
        ("Youngstown", "OH"), ("Lorain", "OH"), ("Hamilton", "OH"), ("Springfield", "OH"),
        # Georgia
        ("Atlanta", "GA"), ("Augusta", "GA"), ("Columbus", "GA"), ("Savannah", "GA"),
        ("Athens", "GA"), ("Sandy Springs", "GA"), ("Roswell", "GA"), ("Macon", "GA"),
        ("Johns Creek", "GA"), ("Albany", "GA"), ("Warner Robins", "GA"), ("Alpharetta", "GA"),
        # Michigan
        ("Detroit", "MI"), ("Grand Rapids", "MI"), ("Warren", "MI"), ("Sterling Heights", "MI"),
        ("Lansing", "MI"), ("Ann Arbor", "MI"), ("Flint", "MI"), ("Dearborn", "MI"),
        ("Livonia", "MI"), ("Troy", "MI"), ("Westland", "MI"), ("Farmington Hills", "MI"),
        # North Carolina (additional)
        ("High Point", "NC"), ("Concord", "NC"), ("Asheville", "NC"), ("Gastonia", "NC"),
        # Arizona
        ("Phoenix", "AZ"), ("Tucson", "AZ"), ("Mesa", "AZ"), ("Chandler", "AZ"),
        ("Scottsdale", "AZ"), ("Glendale", "AZ"), ("Gilbert", "AZ"), ("Tempe", "AZ"),
        ("Peoria", "AZ"), ("Surprise", "AZ"), ("Yuma", "AZ"), ("Flagstaff", "AZ"),
        # Tennessee
        ("Nashville", "TN"), ("Memphis", "TN"), ("Knoxville", "TN"), ("Chattanooga", "TN"),
        ("Clarksville", "TN"), ("Murfreesboro", "TN"), ("Franklin", "TN"), ("Jackson", "TN"),
        ("Johnson City", "TN"), ("Bartlett", "TN"), ("Hendersonville", "TN"), ("Kingsport", "TN"),
        # Indiana
        ("Indianapolis", "IN"), ("Fort Wayne", "IN"), ("Evansville", "IN"), ("South Bend", "IN"),
        ("Carmel", "IN"), ("Fishers", "IN"), ("Bloomington", "IN"), ("Hammond", "IN"),
        ("Gary", "IN"), ("Muncie", "IN"), ("Terre Haute", "IN"), ("Kokomo", "IN"),
        # Missouri
        ("Kansas City", "MO"), ("St. Louis", "MO"), ("Springfield", "MO"), ("Columbia", "MO"),
        ("Independence", "MO"), ("Lee's Summit", "MO"), ("O'Fallon", "MO"), ("St. Joseph", "MO"),
        ("St. Charles", "MO"), ("St. Peters", "MO"), ("Blue Springs", "MO"), ("Florissant", "MO"),
        # Maryland
        ("Baltimore", "MD"), ("Frederick", "MD"), ("Rockville", "MD"), ("Gaithersburg", "MD"),
        ("Bowie", "MD"), ("Annapolis", "MD"), ("College Park", "MD"), ("Salisbury", "MD"),
        ("Laurel", "MD"), ("Greenbelt", "MD"), ("Cumberland", "MD"), ("Hagerstown", "MD"),
        # Wisconsin
        ("Milwaukee", "WI"), ("Madison", "WI"), ("Green Bay", "WI"), ("Kenosha", "WI"),
        ("Racine", "WI"), ("Appleton", "WI"), ("Waukesha", "WI"), ("Oshkosh", "WI"),
        ("Eau Claire", "WI"), ("Janesville", "WI"), ("West Allis", "WI"), ("La Crosse", "WI"),
        # Colorado
        ("Denver", "CO"), ("Colorado Springs", "CO"), ("Aurora", "CO"), ("Fort Collins", "CO"),
        ("Lakewood", "CO"), ("Thornton", "CO"), ("Arvada", "CO"), ("Westminster", "CO"),
        ("Pueblo", "CO"), ("Greeley", "CO"), ("Boulder", "CO"), ("Longmont", "CO"),
        # Minnesota
        ("Minneapolis", "MN"), ("St. Paul", "MN"), ("Rochester", "MN"), ("Duluth", "MN"),
        ("Bloomington", "MN"), ("Brooklyn Park", "MN"), ("Plymouth", "MN"), ("St. Cloud", "MN"),
        ("Eagan", "MN"), ("Woodbury", "MN"), ("Maple Grove", "MN"), ("Eden Prairie", "MN"),
        # South Carolina
        ("Charleston", "SC"), ("Columbia", "SC"), ("North Charleston", "SC"), ("Mount Pleasant", "SC"),
        ("Rock Hill", "SC"), ("Greenville", "SC"), ("Summerville", "SC"), ("Sumter", "SC"),
        ("Hilton Head Island", "SC"), ("Spartanburg", "SC"), ("Florence", "SC"), ("Aiken", "SC"),
        # Alabama
        ("Birmingham", "AL"), ("Montgomery", "AL"), ("Mobile", "AL"), ("Huntsville", "AL"),
        ("Tuscaloosa", "AL"), ("Hoover", "AL"), ("Dothan", "AL"), ("Auburn", "AL"),
        ("Decatur", "AL"), ("Madison", "AL"), ("Florence", "AL"), ("Gadsden", "AL"),
        # Louisiana
        ("New Orleans", "LA"), ("Baton Rouge", "LA"), ("Shreveport", "LA"), ("Lafayette", "LA"),
        ("Lake Charles", "LA"), ("Kenner", "LA"), ("Bossier City", "LA"), ("Monroe", "LA"),
        ("Alexandria", "LA"), ("Houma", "LA"), ("Marrero", "LA"), ("Central", "LA"),
        # Kentucky
        ("Louisville", "KY"), ("Lexington", "KY"), ("Bowling Green", "KY"), ("Owensboro", "KY"),
        ("Covington", "KY"), ("Hopkinsville", "KY"), ("Richmond", "KY"), ("Florence", "KY"),
        ("Georgetown", "KY"), ("Henderson", "KY"), ("Elizabethtown", "KY"), ("Jeffersontown", "KY"),
        # Oregon
        ("Portland", "OR"), ("Eugene", "OR"), ("Salem", "OR"), ("Gresham", "OR"),
        ("Hillsboro", "OR"), ("Bend", "OR"), ("Beaverton", "OR"), ("Medford", "OR"),
        ("Springfield", "OR"), ("Corvallis", "OR"), ("Albany", "OR"), ("Tigard", "OR"),
        # Oklahoma
        ("Oklahoma City", "OK"), ("Tulsa", "OK"), ("Norman", "OK"), ("Broken Arrow", "OK"),
        ("Lawton", "OK"), ("Edmond", "OK"), ("Moore", "OK"), ("Midwest City", "OK"),
        ("Enid", "OK"), ("Stillwater", "OK"), ("Muskogee", "OK"), ("Bartlesville", "OK"),
        # Connecticut
        ("Bridgeport", "CT"), ("New Haven", "CT"), ("Hartford", "CT"), ("Stamford", "CT"),
        ("Waterbury", "CT"), ("Norwalk", "CT"), ("Danbury", "CT"), ("New Britain", "CT"),
        ("West Hartford", "CT"), ("Greenwich", "CT"), ("Hamden", "CT"), ("Meriden", "CT"),
        # Utah
        ("Salt Lake City", "UT"), ("West Valley City", "UT"), ("Provo", "UT"), ("West Jordan", "UT"),
        ("Orem", "UT"), ("Sandy", "UT"), ("Ogden", "UT"), ("St. George", "UT"),
        ("Layton", "UT"), ("Taylorsville", "UT"), ("South Jordan", "UT"), ("Lehi", "UT"),
        # Iowa
        ("Des Moines", "IA"), ("Cedar Rapids", "IA"), ("Davenport", "IA"), ("Sioux City", "IA"),
        ("Iowa City", "IA"), ("Waterloo", "IA"), ("Council Bluffs", "IA"), ("Ames", "IA"),
        ("West Des Moines", "IA"), ("Dubuque", "IA"), ("Ankeny", "IA"), ("Urbandale", "IA"),
        # Nevada
        ("Las Vegas", "NV"), ("Henderson", "NV"), ("Reno", "NV"), ("North Las Vegas", "NV"),
        ("Sparks", "NV"), ("Carson City", "NV"), ("Fernley", "NV"), ("Elko", "NV"),
        ("Mesquite", "NV"), ("Boulder City", "NV"), ("Fallon", "NV"), ("Winnemucca", "NV"),
        # Arkansas
        ("Little Rock", "AR"), ("Fort Smith", "AR"), ("Fayetteville", "AR"), ("Jonesboro", "AR"),
        ("North Little Rock", "AR"), ("Conway", "AR"), ("Rogers", "AR"), ("Pine Bluff", "AR"),
        ("Bentonville", "AR"), ("Hot Springs", "AR"), ("Texarkana", "AR"), ("Benton", "AR"),
        # Mississippi
        ("Jackson", "MS"), ("Gulfport", "MS"), ("Southaven", "MS"), ("Hattiesburg", "MS"),
        ("Biloxi", "MS"), ("Meridian", "MS"), ("Tupelo", "MS"), ("Greenville", "MS"),
        ("Olive Branch", "MS"), ("Horn Lake", "MS"), ("Madison", "MS"), ("Ridgeland", "MS"),
        # Kansas
        ("Wichita", "KS"), ("Overland Park", "KS"), ("Kansas City", "KS"), ("Olathe", "KS"),
        ("Topeka", "KS"), ("Lawrence", "KS"), ("Shawnee", "KS"), ("Manhattan", "KS"),
        ("Lenexa", "KS"), ("Salina", "KS"), ("Hutchinson", "KS"), ("Leavenworth", "KS"),
        # New Mexico
        ("Albuquerque", "NM"), ("Las Cruces", "NM"), ("Rio Rancho", "NM"), ("Santa Fe", "NM"),
        ("Roswell", "NM"), ("Farmington", "NM"), ("Clovis", "NM"), ("Hobbs", "NM"),
        ("Alamogordo", "NM"), ("Carlsbad", "NM"), ("Gallup", "NM"), ("Deming", "NM"),
        # Nebraska
        ("Omaha", "NE"), ("Lincoln", "NE"), ("Bellevue", "NE"), ("Grand Island", "NE"),
        ("Kearney", "NE"), ("Fremont", "NE"), ("Hastings", "NE"), ("North Platte", "NE"),
        ("Norfolk", "NE"), ("Columbus", "NE"), ("Papillion", "NE"), ("La Vista", "NE"),
        # West Virginia
        ("Charleston", "WV"), ("Huntington", "WV"), ("Parkersburg", "WV"), ("Morgantown", "WV"),
        ("Wheeling", "WV"), ("Martinsburg", "WV"), ("Fairmont", "WV"), ("Beckley", "WV"),
        ("Clarksburg", "WV"), ("South Charleston", "WV"), ("St. Albans", "WV"), ("Vienna", "WV"),
        # Idaho
        ("Boise", "ID"), ("Nampa", "ID"), ("Meridian", "ID"), ("Idaho Falls", "ID"),
        ("Pocatello", "ID"), ("Caldwell", "ID"), ("Coeur d'Alene", "ID"), ("Twin Falls", "ID"),
        ("Lewiston", "ID"), ("Post Falls", "ID"), ("Rexburg", "ID"), ("Chubbuck", "ID"),
        # Hawaii
        ("Honolulu", "HI"), ("Hilo", "HI"), ("Kailua", "HI"), ("Kaneohe", "HI"),
        ("Pearl City", "HI"), ("Waipahu", "HI"), ("Kahului", "HI"), ("Ewa Beach", "HI"),
        ("Mililani", "HI"), ("Kihei", "HI"), ("Makakilo", "HI"), ("Kailua-Kona", "HI"),
        # New Hampshire
        ("Manchester", "NH"), ("Nashua", "NH"), ("Concord", "NH"), ("Derry", "NH"),
        ("Rochester", "NH"), ("Salem", "NH"), ("Dover", "NH"), ("Goffstown", "NH"),
        ("Londonderry", "NH"), ("Hudson", "NH"), ("Keene", "NH"), ("Portsmouth", "NH"),
        # Maine
        ("Portland", "ME"), ("Lewiston", "ME"), ("Bangor", "ME"), ("South Portland", "ME"),
        ("Auburn", "ME"), ("Biddeford", "ME"), ("Sanford", "ME"), ("Saco", "ME"),
        ("Augusta", "ME"), ("Westbrook", "ME"), ("Waterville", "ME"), ("Presque Isle", "ME"),
        # Rhode Island
        ("Providence", "RI"), ("Warwick", "RI"), ("Cranston", "RI"), ("Pawtucket", "RI"),
        ("East Providence", "RI"), ("Woonsocket", "RI"), ("Newport", "RI"), ("Central Falls", "RI"),
        ("Westerly", "RI"), ("Cumberland", "RI"), ("North Providence", "RI"), ("Johnston", "RI"),
        # Montana
        ("Billings", "MT"), ("Missoula", "MT"), ("Great Falls", "MT"), ("Bozeman", "MT"),
        ("Butte", "MT"), ("Helena", "MT"), ("Kalispell", "MT"), ("Havre", "MT"),
        ("Anaconda", "MT"), ("Miles City", "MT"), ("Livingston", "MT"), ("Laurel", "MT"),
        # Delaware
        ("Wilmington", "DE"), ("Dover", "DE"), ("Newark", "DE"), ("Middletown", "DE"),
        ("Smyrna", "DE"), ("Milford", "DE"), ("Seaford", "DE"), ("Georgetown", "DE"),
        ("Elsmere", "DE"), ("New Castle", "DE"), ("Laurel", "DE"), ("Harrington", "DE"),
        # South Dakota
        ("Sioux Falls", "SD"), ("Rapid City", "SD"), ("Aberdeen", "SD"), ("Brookings", "SD"),
        ("Watertown", "SD"), ("Mitchell", "SD"), ("Yankton", "SD"), ("Pierre", "SD"),
        ("Huron", "SD"), ("Vermillion", "SD"), ("Spearfish", "SD"), ("Madison", "SD"),
        # North Dakota
        ("Fargo", "ND"), ("Bismarck", "ND"), ("Grand Forks", "ND"), ("Minot", "ND"),
        ("West Fargo", "ND"), ("Williston", "ND"), ("Dickinson", "ND"), ("Mandan", "ND"),
        ("Jamestown", "ND"), ("Wahpeton", "ND"), ("Devils Lake", "ND"), ("Valley City", "ND"),
        # Alaska
        ("Anchorage", "AK"), ("Fairbanks", "AK"), ("Juneau", "AK"), ("Sitka", "AK"),
        ("Ketchikan", "AK"), ("Wasilla", "AK"), ("Kenai", "AK"), ("Kodiak", "AK"),
        ("Bethel", "AK"), ("Palmer", "AK"), ("Homer", "AK"), ("Barrow", "AK"),
        # Vermont
        ("Burlington", "VT"), ("Essex", "VT"), ("South Burlington", "VT"), ("Colchester", "VT"),
        ("Rutland", "VT"), ("Montpelier", "VT"), ("Barre", "VT"), ("St. Albans", "VT"),
        ("Brattleboro", "VT"), ("Milton", "VT"), ("Hartford", "VT"), ("Williston", "VT"),
        # Wyoming
        ("Cheyenne", "WY"), ("Casper", "WY"), ("Laramie", "WY"), ("Gillette", "WY"),
        ("Rock Springs", "WY"), ("Sheridan", "WY"), ("Green River", "WY"), ("Evanston", "WY"),
        ("Riverton", "WY"), ("Jackson", "WY"), ("Cody", "WY"), ("Rawlins", "WY"),
    )
    # US zip range;
    zip_min: int = 10000
    zip_max: int = 99999


@dataclass
class ProductTextConfig:
    adjectives: Tuple[str, ...] = (
        "Ergonomic", "Practical", "Sleek", "Rustic", "Modern", "Compact",
        "Premium", "Luxury", "Classic", "Vintage", "Contemporary", "Elegant",
        "Durable", "Lightweight", "Heavy-Duty", "Portable", "Versatile", "Stylish",
        "Minimalist", "Bold", "Refined", "Sophisticated", "Chic", "Trendy",
        "Professional", "Casual", "Formal", "Sporty", "Comfortable", "Cozy",
        "Eco-Friendly", "Sustainable", "Organic", "Natural", "Handcrafted", "Artisan",
        "Innovative", "Advanced", "Smart", "High-Tech", "Wireless", "Bluetooth",
        "Waterproof", "Weatherproof", "Shockproof", "Stain-Resistant", "Easy-Clean", "Maintenance-Free",
        "Adjustable", "Customizable", "Modular", "Expandable", "Collapsible", "Foldable",
        "Energy-Efficient", "Eco-Conscious", "Recyclable", "Biodegradable", "Non-Toxic", "Safe",
        "Colorful", "Vibrant", "Neutral", "Pastel", "Bold", "Muted",
        "Soft", "Smooth", "Textured", "Glossy", "Matte", "Satin",
    )
    materials: Tuple[str, ...] = (
        "Cotton", "Steel", "Wood", "Plastic", "Leather", "Wool",
        "Bamboo", "Aluminum", "Stainless Steel", "Ceramic", "Glass", "Silk",
        "Polyester", "Nylon", "Canvas", "Denim", "Linen", "Cashmere",
        "Marble", "Granite", "Stone", "Concrete", "Brass", "Copper",
        "Bronze", "Titanium", "Carbon Fiber", "Fiberglass", "Rubber", "Silicone",
        "Cork", "Rattan", "Wicker", "Jute", "Hemp", "Lycra",
        "Spandex", "Mesh", "Fleece", "Felt", "Velvet", "Suede",
        "Vinyl", "Acrylic", "Polycarbonate", "ABS", "TPU", "EVA",
        "Recycled Plastic", "Recycled Metal", "Organic Cotton", "Bamboo Fiber", "Cork Leather", "Mushroom Leather",
        "Titanium Alloy", "Aluminum Alloy", "Stainless Steel", "Brass", "Copper", "Zinc",
        "Oak", "Pine", "Cedar", "Maple", "Walnut", "Cherry",
        "Mahogany", "Teak", "Birch", "Ash", "Elm", "Beech",
    )
    items: Tuple[str, ...] = (
        "Chair", "Mug", "Backpack", "Lamp", "Knife", "Notebook", "Headphones",
        # Furniture
        "Table", "Desk", "Sofa", "Bed", "Cabinet", "Shelf", "Drawer", "Stool", "Bench",
        "Ottoman", "Coffee Table", "Side Table", "Dining Table", "Bookshelf", "Wardrobe",
        # Kitchen Items
        "Pot", "Pan", "Bowl", "Plate", "Cup", "Spoon", "Fork", "Cutting Board",
        "Blender", "Mixer", "Toaster", "Kettle", "Coffee Maker", "Juicer", "Grinder",
        # Electronics
        "Phone", "Tablet", "Laptop", "Monitor", "Keyboard", "Mouse", "Speaker", "Microphone",
        "Camera", "Drone", "Smartwatch", "Fitness Tracker", "Earbuds", "Earphones", "Charger",
        # Clothing & Accessories
        "Shirt", "Pants", "Jacket", "Shoes", "Boots", "Sneakers", "Hat", "Cap",
        "Belt", "Wallet", "Watch", "Sunglasses", "Scarf", "Gloves", "Socks",
        # Bags & Luggage
        "Tote Bag", "Messenger Bag", "Briefcase", "Suitcase", "Duffel Bag", "Gym Bag",
        "Purse", "Handbag", "Clutch", "Crossbody Bag", "Travel Bag", "Laptop Bag",
        # Home Decor
        "Vase", "Candle", "Candle Holder", "Picture Frame", "Mirror", "Clock", "Rug",
        "Curtain", "Pillow", "Blanket", "Throw", "Wall Art", "Decorative Bowl",
        # Office Supplies
        "Pen", "Pencil", "Marker", "Highlighter", "Eraser", "Stapler", "Paper Clip",
        "Binder", "Folder", "File Organizer", "Desk Organizer", "Calendar", "Planner",
        # Sports & Fitness
        "Dumbbell", "Yoga Mat", "Resistance Band", "Jump Rope", "Water Bottle", "Towel",
        "Gym Bag", "Running Shoes", "Bike", "Helmet", "Paddle", "Ball",
        # Tools
        "Hammer", "Screwdriver", "Wrench", "Pliers", "Drill", "Saw", "Level",
        "Tape Measure", "Toolbox", "Flashlight", "Multitool", "Utility Knife",
        # Toys & Games
        "Puzzle", "Board Game", "Card Game", "Action Figure", "Doll", "Building Blocks",
        "Remote Control Car", "Robot", "Drone", "Model Kit", "Art Supplies",
        # Books & Media
        "Book", "E-Reader", "Magazine", "Journal", "Sketchbook", "Photo Album",
        "CD", "DVD", "USB Drive", "Memory Card", "External Hard Drive",
        # Outdoor & Camping
        "Tent", "Sleeping Bag", "Camping Chair", "Cooler", "Flashlight", "Compass",
        "Binoculars", "Telescope", "Hiking Boots", "Walking Stick", "First Aid Kit",
        # Pet Supplies
        "Pet Bed", "Pet Bowl", "Pet Toy", "Pet Leash", "Pet Collar", "Pet Carrier",
        # Baby & Kids
        "Baby Bottle", "Pacifier", "Baby Blanket", "Stroller", "Car Seat", "High Chair",
        "Toy", "Doll", "Action Figure", "Building Blocks", "Puzzle", "Board Game",
    )
    categories: Tuple[str, ...] = (
        "clothing", "kitchen", "accessories", "toys", "electronics", "books", "home", "sports",
        "furniture", "decor", "office", "tools", "outdoor", "camping", "travel", "luggage",
        "fitness", "wellness", "beauty", "personal-care", "health", "medical", "baby", "kids",
        "pet-supplies", "automotive", "garden", "lawn", "hardware", "appliances", "lighting",
        "storage", "organization", "stationery", "art-supplies", "crafts", "hobbies", "music",
        "instruments", "audio", "video", "gaming", "computers", "phones", "tablets",
        "watches", "jewelry", "shoes", "bags", "wallets", "sunglasses", "eyewear",
        "food", "beverages", "snacks", "cooking", "baking", "serving", "dining",
        "bedding", "bath", "towels", "linens", "cleaning", "laundry", "maintenance",
    )
    picture_base_url: str = "https://example.com/images"


@dataclass
class PaymentConfig:
    card_prefixes: Tuple[str, ...] = (
        # Visa test prefixes
        "424242", "401288", "400005", "400000", "400001", "400002", "400003", "400004",
        "400006", "400007", "400008", "400009", "400010", "400011", "400012", "400013",
        "400014", "400015", "400016", "400017", "400018", "400019", "400020", "400021",
        # Mastercard test prefixes
        "555555", "510510", "520082", "520083", "525555", "555555", "510000", "510001",
        "510002", "510003", "510004", "510005", "510006", "510007", "510008", "510009",
        # American Express test prefixes
        "378282", "371449", "378734", "371400", "371401", "371402", "371403", "371404",
        "371405", "371406", "371407", "371408", "371409", "371410", "371411", "371412",
        # Discover test prefixes
        "601111", "601100", "601101", "601102", "601103", "601104", "601105", "601106",
        "601107", "601108", "601109", "601110", "601112", "601113", "601114", "601115",
        # JCB test prefixes
        "353011", "356600", "356601", "356602", "356603", "356604", "356605", "356606",
        "356607", "356608", "356609", "356610", "356611", "356612", "356613", "356614",
        # Diners Club test prefixes
        "305693", "305694", "305695", "305696", "305697", "305698", "305699", "305700",
        "305701", "305702", "305703", "305704", "305705", "305706", "305707", "305708",
        # UnionPay test prefixes
        "620000", "620001", "620002", "620003", "620004", "620005", "620006", "620007",
        "620008", "620009", "620010", "620011", "620012", "620013", "620014", "620015",
        # Generic test prefixes
        "411111", "411112", "411113", "411114", "411115", "411116", "411117", "411118",
        "411119", "411120", "411121", "411122", "411123", "411124", "411125", "411126",
        "411127", "411128", "411129", "411130", "411131", "411132", "411133", "411134",
    )  # common test-like prefixes
    cvv_min: int = 100
    cvv_max: int = 999
    exp_years_ahead_min: int = 1
    exp_years_ahead_max: int = 60


@dataclass
class QuantityConfig:
    quantity_min: int = 1
    quantity_max: int = 1000
    # Skew: probability mass for small quantities
    # (1..3) high prob, rest lower.
    small_quantity_max: int = 3
    small_quantity_prob: float = 0.75


@dataclass
class OutputConfig:
    out_dir: str = field(default_factory=_get_default_output_dir)
    # JSONL files: one JSON object per line
    pretty: bool = False  # if True, writes pretty JSON array instead of JSONL (slower, larger)


@dataclass
class Config:
    seed: int = 1
    # How many payloads per message type: 100k in total.
    counts: Dict[str, int] = field(default_factory=lambda: {
        # Cart service
        "CartItem": 6260,
        "AddItemRequest": 6260,
        "EmptyCartRequest": 3130,
        "GetCartRequest": 3130,
        "Cart": 3130,
        "Empty": 1,
        "EmptyUser": 1565,

        # Recommendation
        "ListRecommendationsRequest": 3130,
        "ListRecommendationsResponse": 3130,

        # Product catalog
        "Product": 6260,
        "ListProductsResponse": 1565,
        "GetProductRequest": 3130,
        "SearchProductsRequest": 2504,
        "SearchProductsResponse": 2504,

        # Shipping
        "GetQuoteRequest": 2504,
        "GetQuoteResponse": 2504,
        "ShipOrderRequest": 2504,
        "ShipOrderResponse": 2504,
        "Address": 4695,

        # Currency
        "Money": 6260,
        "GetSupportedCurrenciesResponse": 626,
        "CurrencyConversionRequest": 2504,

        # Payment
        "CreditCardInfo": 2504,
        "ChargeRequest": 2504,
        "ChargeResponse": 2504,

        # Email
        "OrderItem": 3756,
        "OrderResult": 2504,
        "SendOrderConfirmationRequest": 2504,

        # Checkout
        "PlaceOrderRequest": 2504,
        "PlaceOrderResponse": 2504,

        # Ads
        "AdRequest": 2504,
        "AdResponse": 2504,
        "Ad": 3756,
    })

    dist: DistConfig = field(default_factory=DistConfig)
    money: MoneyConfig = field(default_factory=MoneyConfig)
    address: AddressConfig = field(default_factory=AddressConfig)
    product_text: ProductTextConfig = field(default_factory=ProductTextConfig)
    payment: PaymentConfig = field(default_factory=PaymentConfig)
    qty: QuantityConfig = field(default_factory=QuantityConfig)
    output: OutputConfig = field(default_factory=OutputConfig)

    # Optional: keep some ID pools for slightly more realistic repetition.
    user_pool_size: int = 2000
    product_pool_size: int = 5000
